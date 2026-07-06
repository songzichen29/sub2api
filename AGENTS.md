# Codex 工作准则补充

## 分支约定

- 本仓库的生产分支是 `merge/upstream-main-into-pgsql-mysql-20260426`，不是 `main`。
- 用户要求“合并到生产分支”“生产部署分支”“线上分支”时，默认目标分支必须使用 `merge/upstream-main-into-pgsql-mysql-20260426`。
- 不要因为 `origin/HEAD` 指向 `main` 就把 `main` 当生产分支；合并或推送生产前应先确认当前分支和远端目标分支。

## SQL 迁移文件编码与注释

- 新增或修改 `backend/migrations/**/*.sql` 时，必须使用 **UTF-8 without BOM**。
- 尤其是 MySQL 迁移文件，禁止保存成带 BOM 的 UTF-8；带 BOM 的文件头会变成不可见字符加 `--` 注释，例如 `<BOM>-- comment`，迁移执行器按 `;` 拆分后提交给 MySQL 时，MySQL 可能无法把它识别为注释并报 `Error 1064`。
- 文件开头如需注释，先确认文件首字节就是 `2D 2D`（`--`），不能是 `EF BB BF 2D 2D`。
- 更稳妥做法：MySQL 迁移文件开头直接从第一条 SQL 语句开始，避免文件头注释。
- 写入 SQL / Markdown 等文本文件后，如涉及迁移执行，优先用字节检查确认无 BOM：

```powershell
$b = [IO.File]::ReadAllBytes('backend\migrations\mysql\xxx.sql')
($b[0..([Math]::Min(7, $b.Length-1))] | ForEach-Object { $_.ToString('X2') }) -join ' '
```

正常 SQL 文件不应以 `EF BB BF` 开头。
