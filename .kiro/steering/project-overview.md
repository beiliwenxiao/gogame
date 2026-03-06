# 项目概览

## 项目名称
gogame（模块名：gogame）

## 项目定位
轻量级 MMRPG 游戏引擎，分为后端引擎（gogame）和前端引擎（html5-mmrpg-game）两部分。

- 后端引擎：纯 Go 实现，采用 ECS 架构，支持帧同步与状态同步双模式
- 前端引擎：HTML5 + 原生 JS，基于 Canvas 渲染，代码位于 `cmd/demo/static/src/`
- 两者通过 WebSocket 协议完全解耦通信

## 技术栈

### 后端
- Go 1.25+
- gorilla/websocket — WebSocket 支持
- google/uuid — 实体 ID 生成
- modernc.org/sqlite — SQLite 数据库（纯 Go，无 CGO）
- fsnotify — 配置文件热重载

### 前端
- 原生 HTML5 + JavaScript（ES Module）
- Canvas 2D 渲染
- WebSocket 客户端通信

## 目录结构

```
gogame/
├── internal/           # 后端引擎核心模块
│   ├── config/         # 配置管理
│   ├── monitor/        # 性能监控 & WorkerPool
│   ├── persistence/    # 持久化管理
│   ├── engine/         # 引擎核心：GameLoop、ObjectPool、ECS 类型定义
│   ├── ecs/            # ECS 实体管理（EntityManager）
│   ├── network/        # 网络层：WebSocket/TCP 连接管理
│   ├── codec/          # 消息编解码：JSON + Compact 二进制协议
│   ├── sync/           # 同步系统：帧同步 & 状态同步
│   ├── aoi/            # AOI 兴趣区域：空间索引 & 跨边界管理
│   ├── scene/          # 场景管理
│   ├── map/            # 地图系统：MapManager、A* 寻路、高度图遮挡
│   ├── room/           # 房间管理
│   ├── equipment/      # 装备系统
│   ├── combat/         # 战斗系统
│   └── plugin/         # 插件系统 & EventBus 事件总线
├── proto/
│   └── protocol.md     # 通信协议文档
├── sdk/
│   └── client.ts       # TypeScript 客户端 SDK
├── cmd/demo/           # 修罗斗场 Demo（见 cmd-demo.md）
├── go.mod
└── go.sum
```

## 开发规范

- 后端测试：`go test ./internal/... -count=1`
- 前端代码优先复用 `cmd/demo/static/src/` 中已有的系统和组件
- 新增功能应抽象为可复用的类/函数，放入对应的父模块
- 不要重复造轮子，先查找已有系统（systems/、core/、rendering/ 等）
