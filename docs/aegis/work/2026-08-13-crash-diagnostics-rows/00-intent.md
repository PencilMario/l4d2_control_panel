# Task Intent

## Requested outcome

重做崩溃报告详情中的诊断区域：Stackwalk、AI 诊断、上传元数据和崩溃模块统一为纵向诊断行；Stackwalk 解析为一行一个调用栈帧，同时保留无法识别的原始日志行和原始文件下载。AI 分析继续进入独立 Markdown 阅读页。

## Scope

- Modify the crash report detail UI only.
- Add a pure Stackwalk text parser and render parsed entries.
- Preserve existing crash report API calls, response fields, download actions, analysis reader, and report list behavior.
- Add focused component and parser regression tests.

## Non-goals

- No backend API or persistence changes.
- No changes to AI Markdown format or sanitization policy.
- No changes to Accelerator installation, symbol generation, task execution, or other pages.

## Risk hints

- Real minidump stackwalk output contains both frame lines and explanatory log lines.
- Narrow screens must keep each diagnostic row readable and keep actions reachable.
- Existing focus restoration for the AI reader must remain stable.
