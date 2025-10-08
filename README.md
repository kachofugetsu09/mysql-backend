# 多服务工作区结构

此仓库将用于承载多个 Go 微服务。为避免单体代码堆叠在根目录，将每个服务放置在 `services/<service-name>` 目录下，并通过 `go.work` 管理多个模块的本地联调。

## 当前服务

- `services/mysql-backend`
  - 提供原有的 MySQL 相关接口。
  - 依然使用 `module mysql-backend`，可被其他服务通过 `go.work` 本地引用。

## 开发说明

1. 进入目标服务目录，例如：
   ```bash
   cd services/mysql-backend
   go run ./...
   ```
2. 若新增微服务，请在 `services/` 下创建对应模块，并在根目录 `go.work` 中添加 `use ./services/<service-name>`。

这样可以在同一仓库中维护多个服务，同时保持目录结构清晰、版本控制友好。
