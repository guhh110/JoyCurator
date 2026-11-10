package recognition

type Store interface {
	LoadRules() (Pipeline, error)
	SaveRules(Pipeline) error
	StartTrace(Trace) error
	FinishTrace(Trace, []Step) error
	TraceHistory(int, string, string) ([]Trace, int, error)
	TraceDetail(string) (Trace, []Step, error)
	DeleteTrace(string) error
}
