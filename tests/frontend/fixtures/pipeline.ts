import type {
  Rule,
  Pipeline,
  ExtractionResult,
} from '../../../frontend/src/features/extraction/model'
export const annotation = (id = 'title', order = 10): Rule => ({
  id,
  name: id,
  stage: 'annotate',
  target: 'title',
  source: 'all',
  order,
  enabled: true,
  kind: 'script',
  config: { code: 'function extract(ctx) { return [] }' },
  examples: [],
})
export function pipeline(annotators: Rule[] = [annotation()]): Pipeline {
  return {
    tokenizer: {
      id: 'tokenizer',
      name: '分词',
      stage: 'tokenize',
      order: 0,
      enabled: true,
      kind: 'script',
      config: { code: 'function extract(ctx) { return [] }' },
      examples: [],
    },
    annotators,
    resolver: {
      id: 'resolver',
      name: '元数据裁决',
      stage: 'resolve',
      order: 0,
      enabled: true,
      kind: 'script',
      config: { code: 'function extract(ctx) { return [] }' },
      examples: [],
    },
  }
}
export function extraction(title = 'Film'): ExtractionResult {
  const metadata = {
    tmdbId: null,
    title,
    mediaType: null,
    year: null,
    season: null,
    episodes: [],
    releaseTag: null,
  }
  return {
    context: {
      contextRoot: '/nas',
      sourceBoundary: '/',
      paths: [`/nas/${title}.mkv`],
      targets: [`/nas/${title}.mkv`],
    },
    tree: [],
    documents: [
      {
        fileId: 'file-0',
        path: `/nas/${title}.mkv`,
        directory: '/nas',
        sources: [],
        tokens: [],
        candidates: [],
      },
    ],
    files: [
      { fileId: 'file-0', path: `/nas/${title}.mkv`, metadata, decisions: {} },
    ],
    steps: [],
  }
}
