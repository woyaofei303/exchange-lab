# Exchange Lab

一个用于学习和实践交易系统架构的个人工程项目。

目标是通过 Go、MySQL、Redis、Kafka、Docker、
可观测性和 AI 技术，构建一个简化版交易平台。

该项目重点关注：

- 后端服务设计
- 数据一致性
- 交易流程
- 系统稳定性
- 工程化部署
- AI 辅助运维


---

# Project Goal

通过模拟交易系统，学习完整的软件交付流程：

用户请求：

Frontend
↓
Backend API
↓
Database
↓
Cache
↓
Message Queue
↓
Monitoring
↓
AI Assistant


最终实现：

- 用户系统
- 账户系统
- 资产系统
- 订单系统
- 撮合系统
- 账本系统
- 监控系统
- AI 故障分析助手


---

# Tech Stack

## Frontend

- TypeScript
- React / Vue

## Backend

- Go
- Gin

## Database

- MySQL

## Cache

- Redis

## Message Queue

- Kafka

## Deployment

- Linux
- Nginx
- Docker Compose

## CI/CD

- GitLab CI

## Observability

- OpenTelemetry
- Prometheus
- Grafana

## AI

- Python
- LLM API
- Tool Calling
- RAG
- Evals


---

# System Architecture

(后续补充架构图)


---

# Features

## User

- [ ] 用户注册
- [ ] 用户登录
- [ ] 权限管理


## Account

- [ ] 用户账户
- [ ] 资产查询
- [ ] 余额变化


## Trading

- [ ] 创建订单
- [ ] 撤销订单
- [ ] 订单状态管理
- [ ] 简化撮合


## System

- [ ] Redis缓存
- [ ] Kafka消息
- [ ] 日志系统
- [ ] 链路追踪
- [ ] 监控告警


## AI

- [ ] AI日志分析
- [ ] AI故障排查
- [ ] AI运行手册查询


---

# Project Progress


## Phase 0 Environment Setup

Completed:

- [x] Go environment
- [x] Docker environment
- [x] MySQL container
- [x] GitHub repository
- [x] SSH authentication


## Phase 1 Backend Foundation

TODO:

- [ ] Go HTTP Server
- [ ] User API
- [ ] Database connection
- [ ] Unit Test


---

# Local Development


## Requirements

- Go
- Docker
- Docker Compose


## Start MySQL

```bash
docker compose up -d