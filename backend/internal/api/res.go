package api

type BaseRes struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data"`
}

func NewBaseRes(code int, msg string, data interface{}) *BaseRes {
	return &BaseRes{
		Code: code,
		Msg:  msg,
		Data: data,
	}
}

func NewSuccessRes(data interface{}) *BaseRes {
	return NewBaseRes(0, "success", data)
}

var (
	SUCCESS = 0
	// 通用与细分错误码（可按需扩展）
	CodeBadRequest     = 40000
	CodeImageRequired  = 40001
	CodeScanFailed     = 50001
	CodeUpdateFailed   = 50002
	CodeDockerError    = 50003
	CodeRegistryError  = 50004
	CodeInvalidRequest = 50005
)

// NewErrorResCode 返回带自定义错误码的响应
func NewErrorResCode(code int, msg string) *BaseRes {
	return NewBaseRes(code, msg, nil)
}
