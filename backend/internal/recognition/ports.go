package recognition

type Store interface {
	LoadRules() ([]Rule, error)
	SaveRules([]Rule) error
	StartTrace(Trace) error
	AppendStep(string, int, Step) error
	FinishTrace(Trace) error
	TraceHistory(int, string, string) ([]Trace, int, error)
	TraceDetail(string) (Trace, []Step, error)
	DeleteTrace(string) error
}
