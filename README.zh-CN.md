# gh-ost for OceanBase

[English](README.md) | 简体中文

基于 [GitHub gh-ost](https://github.com/github/gh-ost) 的 OceanBase MySQL 模式社区兼容版，
延续已有的 [whhe/gh-ost](https://github.com/whhe/gh-ost) OB 适配并持续整合官方更新。

## 核心价值：让部分 OB Offline DDL 可以在线迁移

**不必在整个 DDL 执行期间禁止业务写入。**

对已验证且满足前提条件的 DDL 操作，执行期间业务可以继续 SELECT、INSERT、UPDATE、DELETE；
**最终的元数据切换阶段（即由 gh-ost 影子表替换原表的过程）仍需极短时间的写锁，期间会短暂阻塞写入，并非完全无感。**

### 哪些场景验证过？

| 类别 | 此前验证通过的具体场景 |
| --- | --- |
| 列操作 | AFTER/FIRST 重排列；添加自增列；已有非自增整数主键改为自增；添加 STORED 生成列；删除带普通索引的列；增大 DECIMAL 总精度且小数位不变、跨存储位宽 |
| 主键操作 | 无主键但有合适 UNIQUE NOT NULL 键时添加主键；另有共享 UNIQUE NOT NULL 键时仅删除主键约束 |
| 字符集 | 转换已有字符列的字符集 |
| 存储格式 | 行存转列存、行存转行列混存、行列混存转列存、行列混存转行存，均为立即转换、不带 DELAYED |
| 分区 | 修改分区规则；修改自动分区属性并改变分区规则 |

## 在 OceanBase 上使用

### 1. 前提条件

- 使用 OB **MySQL 模式**。Oracle 模式不在本文范围内。
- 已部署并配置与目标租户对应、gh-ost 可访问的 MySQL 兼容 binlog 服务。
- 原表和目标表必须共享 gh-ost 可用的稳定主键或 UNIQUE NOT NULL 键。
  无主键并不一定不能迁移，但不能没有合适的共享唯一键。
- 账号具备建表、ALTER、数据读写、删除表、LOCK TABLES。
- OB 参数 `enable_lock_priority` 必须为 `true`，并支持本适配使用的锁表及 RENAME 行为。
  工具会检查此前提；参数不满足时，应由管理员评估配置。
- 预留影子表、索引和保留旧表所需的空间，评估复制、binlog 回放对 CPU、IO 和日志保留的影响。
  外键、触发器等上游限制仍需检查。

### 2. 下载 OB 兼容版

从[本仓库 Releases](https://github.com/davidzhangbj/gh-ost/releases)
下载与你的操作系统和 CPU 架构对应的二进制包。

以下以 Linux x86-64 为例，在下载目录执行：

```sh
tar -xzf gh-ost-linux-amd64.tar.gz
./gh-ost --version
```

### 3. 示例：添加自增列

假设测试表已有稳定主键 `id`，没有自增列，并包含 `v`、`pad` 两个普通列。
先在测试环境通过原生 SQL 创建示例表，再使用 gh-ost 迁移：

```sql
CREATE TABLE demo (
  id BIGINT NOT NULL PRIMARY KEY,
  v BIGINT NOT NULL,
  pad VARCHAR(32) NOT NULL
);
INSERT INTO demo VALUES (1, 10, 'first'), (2, 20, 'second');
```

将下列连接占位值替换为你的测试环境配置。`--ask-pass` 会交互读取密码，
避免把密码放到命令参数、shell 历史或文档中。此命令包含 `--execute`，**会真实修改表结构**：

```sh
./gh-ost \
  --host=YOUR_OB_BINLOG_CAPABLE_ENDPOINT \
  --port=3306 \
  --user=YOUR_MIGRATION_USER \
  --database=YOUR_DATABASE \
  --table=demo \
  --alter="ADD COLUMN c BIGINT AUTO_INCREMENT" \
  --ask-pass \
  --assume-rbr \
  --chunk-size=1000 \
  --max-lag-millis=3000 \
  --execute
```

### 4. 示例：AFTER 重排列

对于同样包含 `id`、`v`、`pad` 的表，沿用上述连接和执行参数，将 `--alter` 替换为：

```sh
--alter="MODIFY COLUMN pad VARCHAR(32) NOT NULL AFTER id"
```

### 5. 生产使用前的检查

- 先完成备份、恢复演练及同版本同结构的迁移验证。
- 根据业务写入速率调节迁移负载并监控追增量进度。本适配会忽略 `--max-load` 和
  `--critical-load`，不能依赖它们保护 OB；应结合 OB 监控、延迟和容量控制。
- 持续写入超过回放能力时，增量队列可能挤占全量复制。
- 需要人工控制切换时，可使用
  [`--postpone-cut-over-flag-file`](doc/interactive-commands.md)，并为切换预留短暂写阻塞窗口。
- 默认不自动删除迁移后的旧表。不要在迁移仍运行时手工清理 `_gho`、`_ghc` 等辅助表。
  保留旧表也不等于自动回滚：切换后的新写入不会自动同步回旧表。

---

## 上游 gh-ost 项目介绍

以下为上游项目介绍的中文说明。在 OceanBase 上应以前述使用条件和验证范围为准。
原始英文介绍保留在 [英文 README](README.md#upstream-gh-ost-documentation) 中。

### GitHub 的 MySQL 在线表结构迁移工具

`gh-ost` 是一款不使用触发器的 MySQL 在线表结构迁移工具，支持迁移测试、暂停、
动态控制与配置调整、审计，以及其他运维能力。它将迁移写入与业务负载解耦，
并提供对迁移过程的控制。

### 工作原理

在线表结构迁移通常先创建与原表相似的影子表，在空影子表上执行结构变更，
再分批复制原表数据，同时将业务 INSERT、DELETE、UPDATE 同步到影子表，
最后在合适时机用影子表替换原表。

`gh-ost` 采用同样的基本流程，但不使用触发器，而是通过 binlog 捕获变更并异步应用。
触发器存在一些[限制和风险](doc/why-triggerless.md)；binlog 驱动的方式让工具可以
控制复制与增量应用，支持暂停迁移写入和动态调节迁移负载。
详见[无触发器设计](doc/triggerless-design.md)和[运维能力](doc/perks.md)。

![gh-ost 迁移流程](doc/images/gh-ost-general-flow.png)

### 主要能力

- **副本测试：** 上游提供在副本上执行迁移并保留两张表进行比较的模式，便于持续验证。
  这不表示 OB 已验证相同的副本工作流。
- **暂停迁移写入：** 节流时暂停行复制和增量应用，减轻迁移带来的写入负载。
- **动态控制：** 运行期间可以调整参数或手动触发节流。
- **状态审计：** 通过 Unix socket 或 TCP 查询迁移状态。
- **控制切换：** 可延迟最终表切换，等到运维人员准备就绪后再执行。
- **外部集成：** 使用 [hooks](doc/hooks.md) 与现有运维环境集成。

更多内容见 [doc](doc) 目录。上游文档主要为英文。

### 通用用法与文档

[命令速查](doc/cheatsheet.md)介绍了上游的多种运行方式，包括 noop 检查、
通过副本迁移主库、直接迁移主库、在副本执行迁移以及副本测试。
**这些 MySQL 工作流不能直接视为已验证的 OB 工作流。**

上游建议先测试、再执行；正式迁移前先运行 noop 检查，再通过 `--execute` 执行。
noop 并不代表生产迁移或数据正确性已得到验证。其他常用选项包括：

- `--exact-rowcount`：获取更准确的进度估算。
- `--postpone-cut-over-flag-file`：控制最终切换时机。
- [交互命令](doc/interactive-commands.md)：运行期间查询状态和调整参数。

相关文档：

- [要求与限制](doc/requirements-and-limitations.md)
- [共享迁移键](doc/shared-key.md)
- [常见问题](doc/questions.md)
- [异常情况](doc/what-if.md)
- [详细注意事项](doc/the-fine-print.md)
- [副本测试](doc/testing-on-replica.md)
- [AWS RDS](doc/rds.md) 与 [Azure Database for MySQL](doc/azure.md)
- [上游社区问题](https://github.com/github/gh-ost/issues?q=label%3Aquestion)

### 名称由来

项目最初名为 `gh-osc`，意为 GitHub Online Schema Change，后来将 `c` 换成 `t`，
成为 `gh-ost`，读作 Ghost。上游以 Transmogrifier、Translator、Transformer、
Transfigurator 等词解释这个缩写。

### 许可证与社区

`gh-ost` 使用 [MIT 许可证](LICENSE)。第三方依赖各自遵循其许可证，详见 [vendor](vendor)。
上游项目由 GitHub 数据库基础设施团队设计、开发、评审和测试，原作者名单见
[英文 README](README.md#authors)。本仓库保留对上游和既有 OB 适配作者的归属说明。

上游贡献流程见 [CONTRIBUTING](.github/CONTRIBUTING.md) 和
[开发指南](doc/coding-ghost.md)。OB 兼容版的问题请提交到
[本仓库 Issues](https://github.com/davidzhangbj/gh-ost/issues)，不要将 fork 特有的问题
直接当作官方 gh-ost 已确认的问题。

