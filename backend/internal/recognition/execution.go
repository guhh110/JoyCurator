package recognition

import (
	"context"
	"fmt"
	"runtime"
	"sync"
)

type ruleEmitter func(compiledRule, *Document, map[string]any, func(any) (any, error)) error

func lightEmitter(ctx context.Context, diagnostics *[]Diagnostic) ruleEmitter {
	return func(c compiledRule, d *Document, values map[string]any, apply func(any) (any, error)) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		r := c.rule
		if !r.Enabled {
			return nil
		}
		m, err := c.run(ctx, values, d, false)
		if err == nil {
			_, err = apply(m.value)
		}
		if err != nil {
			diagnostic := Diagnostic{Stage: r.Stage, RuleID: r.ID, Message: err.Error(), Line: m.line, Column: m.column}
			if d != nil {
				diagnostic.FileID = d.FileID
			}
			*diagnostics = append(*diagnostics, diagnostic)
		}
		return err
	}
}

// Both debug and production use this core. Only observation and scheduling differ.
func (e *Engine) processDocument(ctx context.Context, d *Document, emit ruleEmitter, observerFailure *error) ([]error, error) {
	err := emit(e.tokenizer, d, map[string]any{"file": fileIdentity(*d), "sources": d.Sources}, func(v any) (any, error) {
		tokens, err := resultArray(v, resultToken)
		if err != nil {
			return nil, err
		}
		if err = validateTokens(*d, tokens); err != nil {
			return nil, err
		}
		d.Tokens = tokens
		return tokens, nil
	})
	if err != nil {
		return nil, err
	}
	var failures []error
	for _, c := range e.annotators {
		err = emit(c, d, map[string]any{"file": fileIdentity(*d), "sources": d.Sources, "tokens": d.Tokens, "candidates": d.Candidates}, func(v any) (any, error) {
			items, err := candidates(*d, c.rule, v)
			if err == nil {
				d.Candidates = append(d.Candidates, items...)
			}
			return items, err
		})
		if observerFailure != nil && *observerFailure != nil {
			return failures, *observerFailure
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", c.rule.ID, err))
		}
		if ctx.Err() != nil {
			return failures, ctx.Err()
		}
	}
	return failures, nil
}

type documentWork struct {
	index       int
	doc         Document
	diagnostics []Diagnostic
	failures    []error
	fatal       error
}
type documentJob struct {
	index int
	ctx   context.Context
}

// A window of at most four documents bounds speculative work and memory. Workers
// own their runtimes and mutable documents; only this coordinator merges results
// and calls progress, in the original sorted order.
func (e *Engine) processDocuments(ctx context.Context, documents []Document, target map[string]bool, total int, progress Progress, diagnostics *[]Diagnostic) ([]error, error) {
	workers := min(4, runtime.GOMAXPROCS(0), len(documents))
	workCtx, cancel := context.WithCancel(ctx)
	jobs := make(chan documentJob, workers)
	results := make(chan documentWork, workers)
	var group sync.WaitGroup
	for n := 0; n < workers; n++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for {
				select {
				case <-workCtx.Done():
					return
				case job, ok := <-jobs:
					if !ok {
						return
					}
					result := documentWork{index: job.index, doc: documents[job.index]}
					result.failures, result.fatal = e.processDocument(job.ctx, &result.doc, lightEmitter(job.ctx, &result.diagnostics), nil)
					select {
					case results <- result:
					case <-workCtx.Done():
						return
					}
				}
			}
		}()
	}
	defer func() { cancel(); close(jobs); group.Wait() }()
	active := make(map[int]context.CancelFunc, workers)
	dispatch := func(index int) {
		jobCtx, stop := context.WithCancel(workCtx)
		active[index] = stop
		jobs <- documentJob{index: index, ctx: jobCtx}
	}
	scheduled := 0
	for scheduled < workers {
		dispatch(scheduled)
		scheduled++
	}
	pending := make(map[int]documentWork, workers)
	var failures []error
	fatalIndex := len(documents)
	next, completed := 0, 0
	for next < len(documents) {
		select {
		case <-ctx.Done():
			return failures, ctx.Err()
		case result := <-results:
			active[result.index]()
			delete(active, result.index)
			if result.fatal != nil && result.index < fatalIndex {
				fatalIndex = result.index
				// Stop dispatch immediately. Retain earlier in-flight files so the
				// committed prefix and diagnostics do not depend on completion order.
				for index, stop := range active {
					if index > fatalIndex {
						stop()
					}
				}
			}
			pending[result.index] = result
		}
		for {
			result, ok := pending[next]
			if !ok {
				break
			}
			delete(pending, next)
			documents[next] = result.doc
			*diagnostics = append(*diagnostics, result.diagnostics...)
			failures = append(failures, result.failures...)
			if result.fatal != nil {
				return failures, result.fatal
			}
			if target[result.doc.Path] {
				completed++
				if progress != nil {
					if err := progress(completed, total); err != nil {
						return failures, err
					}
				}
			}
			next++
			if scheduled < fatalIndex {
				dispatch(scheduled)
				scheduled++
			}
		}
	}
	return failures, ctx.Err()
}
