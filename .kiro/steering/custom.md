---
inclusion: always
---

# 用户约束

- 请使用中文回答
- 执行 task 时，当完成一个小任务后，继续往后执行，拥有所有权限
- 不要主动创建新的文档和总结
- 所有功能，优先使用和复用已有系统的功能
- 不要每次都写文档。除非用户要求，否则不要写文档
- 没有用户同意，不允许创建测试页面
- 调试完成后，如果要删除调试信息，要先询问用户是否同意删除，不要自动删除
- gfgame是后端引擎，html5-mmrpg-game是前端引擎
- cmd/demo中，需要包含html5-mmrpg-game的代码，尽量采用html5-mmrpg-game的代码
- cmd/demo的前端，尽量使用html5-mmrpg-game/src/prologue中的代码，不要自己另外写。
- 添加和修改前端页面时，先参考一下 #html5-mmrpg-game/docs/usage_guide.md
如果已经有功能，则复用。如果没有功能，则写出功能后，抽象成好调用的函数或类，放到父模块中。包括cmd/demo中调用的js，以及html5-mmrpg-game中的js。