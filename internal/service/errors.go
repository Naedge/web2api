package service

type StatusError struct {
	Code    int
	Message string
}

func (e *StatusError) Error() string {
	return e.Message
}

func badRequest(message string) error {
	return &StatusError{Code: 400, Message: message}
}

func BadRequest(message string) error {
	return badRequest(message)
}

func unauthorized(message string) error {
	return &StatusError{Code: 401, Message: message}
}

func notFound(message string) error {
	return &StatusError{Code: 404, Message: message}
}

func badGateway(message string) error {
	return &StatusError{Code: 502, Message: message}
}
