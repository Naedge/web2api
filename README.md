# web2api

用 Go 重写 `basketikun/chatgpt2api`，使用 `gin` 和 `gorm`，并直接接入原仓库 `web` 前端静态导出。

## 运行

1. 复制 `config.json.example` 为 `config.json`
2. 执行 `go mod tidy`
3. 执行 `go run ./cmd/web2api`
4. 浏览器打开 `http://127.0.0.1:8080/`

## 登录

- 首次进入会显示初始化界面，先创建管理员账号和密码
- 管理后台接口使用 Cookie 会话登录
- OpenAI 兼容接口 `/v1/*` 继续使用 `api-key`
- 前端源码保留在 `web/`，当前只对登录方式做了账号密码改造

## 目录

- `cmd/web2api`: 程序入口
- `internal/config`: 配置加载
- `internal/frontend`: 内嵌前端资源
- `internal/model`: GORM 模型
- `internal/repository`: 数据访问层
- `internal/service`: 业务层
- `internal/handler`: HTTP 处理层
- `internal/router`: Gin 路由
- `web`: 原仓库前端源码

## 主要接口

- `GET /`
- `GET /auth/status`
- `POST /auth/setup`
- `POST /auth/login`
- `POST /v1/images/generations`
- `POST /v1/images/edits`
- `POST /v1/chat/completions`
- `POST /v1/responses`
- `GET /api/accounts`
- `POST /api/accounts`
- `POST /api/accounts/update`
- `POST /api/accounts/refresh`
- `GET /api/cpa/pools`
- `POST /api/cpa/pools`
- `GET /api/cpa/pools/:poolID/files`
- `POST /api/cpa/pools/:poolID/import`
