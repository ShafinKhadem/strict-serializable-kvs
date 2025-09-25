package kvs

type PutRequest struct {
	Key   string
	Value string
	TransactionID string
}

type PutResponse struct {
	Success  bool
    LockFail bool
}

type GetRequest struct {
	Key string
	TransactionID string
}

type GetResponse struct {
	Value string
	Success  bool
	LockFail bool
}

type AbortRequest struct {
    TransactionID string
}

type CommitRequest struct {
    TransactionID string
    Lead          bool    // the first participant is the lead
}

type CommitResponse struct {
    Success bool
}

type AbortResponse struct {
    Success bool
}