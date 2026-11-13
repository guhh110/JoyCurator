package recognition

type Store interface {
	LoadRules() (Pipeline, error)
	SaveRules(Pipeline) error
	StartTrace(Trace) error
	FinishTrace(Trace, Snapshot) error
	TraceHistory(int, string, string) ([]Trace, int, error)
	TraceDetail(string) (Trace, Snapshot, error)
	DeleteTrace(string) error
}
