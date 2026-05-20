@echo off
cd /d "%~dp0"
if exist "build\bin\jcp.exe" (
    echo 正在启动韭菜盘...
    start "" "build\bin\jcp.exe"
) else (
    echo ========================================================
    echo  提示：尚未编译正式版软件。
    echo  请在当前目录打开终端，运行以下命令进行编译：
    echo    wails build
    echo  编译完成后，双击此脚本或直接运行 build\bin\jcp.exe 即可。
    echo ========================================================
    pause
)
