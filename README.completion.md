# Shell 自动补全（`kopsc completion`）

> 返回 [README.md](README.md)

为 bash、zsh、fish 或 powershell 生成命令补全脚本，配置后可通过 Tab 键自动补全命令、子命令和选项。

```bash
kopsc completion bash        # 生成 bash 补全脚本
kopsc completion zsh         # 生成 zsh 补全脚本
kopsc completion fish        # 生成 fish 补全脚本
kopsc completion powershell  # 生成 powershell 补全脚本
```

## 配置方法

### Bash（Linux）

**临时生效（当前会话）：**

```bash
source <(kopsc completion bash)
```

**永久生效：**

```bash
# 将补全脚本添加到 /etc/bash_completion.d/
kopsc completion bash | sudo tee /etc/bash_completion.d/kopsc > /dev/null

# 重新加载配置
source ~/.bashrc
```

### Zsh（macOS / Linux）

**临时生效（当前会话）：**

```zsh
source <(kopsc completion zsh)
```

**永久生效：**

```zsh
# 将补全脚本添加到 ~/.zshrc
echo 'source <(kopsc completion zsh)' >> ~/.zshrc

# 重新加载配置
source ~/.zshrc
```

或者使用更规范的方式：

```zsh
# 创建补全目录
mkdir -p ~/.zsh/completions

# 生成补全脚本
kopsc completion zsh > ~/.zsh/completions/_kopsc

# 在 ~/.zshrc 中添加（如果还没有）
echo 'fpath=(~/.zsh/completions $fpath)' >> ~/.zshrc
echo 'autoload -U compinit && compinit' >> ~/.zshrc

# 重新加载配置
source ~/.zshrc
```

### Fish

**临时生效（当前会话）：**

```fish
kopsc completion fish | source
```

**永久生效：**

```fish
# 将补全脚本添加到 ~/.config/fish/completions/
kopsc completion fish > ~/.config/fish/completions/kopsc.fish

# 重新加载配置（或重启 fish）
source ~/.config/fish/config.fish
```

### PowerShell（Windows）

**临时生效（当前会话）：**

```powershell
kopsc completion powershell | Out-String | Invoke-Expression
```

**永久生效：**

```powershell
# 生成补全脚本到文件
kopsc completion powershell > "$env:USERPROFILE\Documents\WindowsPowerShell\kopsc_completion.ps1"

# 在 PowerShell 配置文件中添加（用 notepad $PROFILE 编辑）
Add-Content -Path $PROFILE -Value ". $env:USERPROFILE\Documents\WindowsPowerShell\kopsc_completion.ps1"

# 重新加载配置
. $PROFILE
```

如果 `$PROFILE` 文件不存在，先创建：

```powershell
New-Item -Type File -Force $PROFILE
```

## 验证配置

配置完成后，尝试以下操作：

```bash
# 输入 kopsc 后按 Tab，应显示所有分组
kopsc <Tab>

# 输入 kopsc docker 后按 Tab，应显示 docker 子命令
kopsc docker <Tab>

# 输入 kopsc file findbig -- 后按 Tab，应显示选项
kopsc file findbig --<Tab>
```

## 注意事项

- 补全脚本会列出所有已注册的命令和选项
- 如果新增命令后补全不生效，重新执行上述配置步骤即可
- PowerShell 可能需要调整执行策略：`Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser`
