package core

/*
Communication is an enumeration for the available communication methods
of Tinzenite peers.
*/
type Communication int

const (
	/*CmNone method.*/
	CmNone Communication = iota
	/*CmTox protocol.*/
	CmTox
)

func (cm Communication) String() string {
	switch cm {
	case CmNone:
		return "None"
	case CmTox:
		return "Tox"
	default:
		return "unknown"
	}
}

/*
Operation is the enumeration for the possible protocol operations.
*/
type Operation int

const (
	/*OpUnknown operation.*/
	OpUnknown = iota
	/*OpCreate operation.*/
	OpCreate
	/*OpModify operation.*/
	OpModify
	/*OpRemove operation.*/
	OpRemove
)

func (op Operation) String() string {
	switch op {
	case OpCreate:
		return "create"
	case OpModify:
		return "modify"
	case OpRemove:
		return "remove"
	default:
		return "unknown"
	}
}

/*
Request defines what has been requested.
*/
type Request int

const (
	/*ReNone is default empty request.*/
	ReNone Request = iota
	/*ReObject requests an object.*/
	ReObject
	/*ReModel requests the model.*/
	ReModel
)

func (req Request) String() string {
	switch req {
	case ReNone:
		return "None"
	case ReObject:
		return "Object"
	case ReModel:
		return "Model"
	default:
		return "unknown"
	}
}
