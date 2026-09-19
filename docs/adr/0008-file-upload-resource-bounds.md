# 文件上传请求资源边界

Status: accepted

当前文件上传 Handler 在调用 `FormFile` 或 `MultipartForm` 后才进入 Service/Storage，而 Go 的 multipart 解析会先读取整个请求体；因此 Storage 的 50 MiB 单文件限制不能防止超大请求消耗网络、内存和临时磁盘。文件上传采用分层资源边界：单文件内容最多 50 MiB，批量最多 20 个文件且文件内容总量最多 200 MiB；单文件和批量接口的原始 HTTP body 上限分别为 55 MiB 和 210 MiB，所有 multipart 接口必须显式登记策略，未登记接口直接拒绝。

请求体上限必须在 multipart 解析前生效，并同时覆盖带 `Content-Length` 和 chunked 请求；上传接口采用流式 multipart 解析，按文件数、单文件内容、批次内容总量以及表单字段/文件名字节数即时计数。由于图片内容校验需要可回退读取，单文件可以使用受控的临时暂存，但不得让 `ParseMultipartForm` 无界解析整个批次；Storage 的 50 MiB 校验继续作为最终防线。

上传资源还受全局最多 8 个进行中 multipart 请求、10 秒请求头读取超时和 5 分钟上传读取时长限制；反向代理的限制不得高于应用限制。body、文件数或批次总大小超限统一返回 HTTP 413 与业务 `code=413`；批量请求中单个文件超限仍保留既有 HTTP 200、业务 `code=400` 的逐文件失败契约，multipart 格式错误返回 HTTP 400。

选择这些边界是为了在保持合法批量上传和既有部分成功行为的同时，阻断“先完整解析、后由 Storage 拒绝”的资源耗尽路径。验收必须覆盖超大 `Content-Length`、超大 chunked body、文件数/批次大小/单文件/字段字节数超限、并发上传、临时文件清理和超限观测；实现后同步更新 Swagger 与上传需求文档。
