# ADR-009：模块化单体加独立 worker，而非过早微服务化

状态：Accepted

## 决策

领域 package、HTTP adapter、PostgreSQL repository 和 worker 共用一个 Go module/镜像，通过不同 `cmd/*` 进程部署。跨事务异步边界只在有明确交付语义的 Outbox/JetStream 使用。

## 原因与代价

一个人学习和交付时，模块化单体能降低构建、版本、契约和本地运维成本，同时保留进程隔离。代价是共享发布节奏和镜像体积；需要 package boundary、接口和测试避免耦合。

## 演进与回滚

只有当 profiling/SLO 证明独立扩容或故障隔离收益时，才沿现有 event/repository boundary 拆服务。拆分必须先定义数据所有权和恢复流程；回滚保持相同事件版本与数据库兼容。
