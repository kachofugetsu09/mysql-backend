package agent

// QueryRequest 表示 RPC 查询请求参数
type QueryRequest struct {
	Query string `json:"query"`
}

// QueryResponse 表示 RPC 查询响应
type QueryResponse struct {
	Answer string `json:"answer"`
}

// Service 暴露给 mysql-backend 的 RPC 服务
type Service struct{}

// Query 返回一个固定的 helloworld 响应，用于连通性测试
func (s *Service) Query(req QueryRequest, resp *QueryResponse) error {
	query := req.Query
	resp.Answer = query + "helloworld"
	return nil
}
