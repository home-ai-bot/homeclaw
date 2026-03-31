1. 前置准备
    1. 解压执行程序包
    2. 找到 ffmpeg https://ffmpeg.org/download.html、go2rtc https://github.com/AlexxIT/go2rtc/releases 对应的系统的可执行程序，放到 picoclaw-launcher 同目录下目录结构类似：
        ```
        D:\claw
        ├── LICENSE
        ├── ffmpeg.exe
        ├── go2rtc.exe
        ├── picoclaw-launcher-tui.exe
        ├── picoclaw-launcher.exe
        └── picoclaw.exe
        ```
    3. 执行picoclaw-launcher
    4. 会自动打开浏览器，访问http://localhost:18800
2. 模型配置
    1. 在页面上配置要使用的模型
    如使用本地ollama参考：
    {
      "model_name": "ollama-qwen-small",
      "model": "ollama/sorc/qwen3.5-claude-4.6-opus-q4:4b",
      "api_base": "http://127.0.0.1:11434/v1",
      "api_key": "local"
    }
3. 小米账号配置
    1. 访问 http://localhost:1984 或 http://{yourIP}:1984, 点击上方add，找到xiaomi，按照指引登录
    3. 登录完成后，可以列出相关摄像头设备
    3. 点击上方config，等待刷新出config的内容后尤其是xiaomi 账号和token，点击save&Restart
    4. 访问http://localhost:18800，点击上方第2个按钮，重启视频服务器 go2rtc
4. 同步&控制小米设备
    1. 在聊天框，输入同步小米设备，则homeclaw自动化执行设备同步
    2. 设备同步完毕后，可以说：打开某个开关，关闭某个开关
    3. 如果有摄像头，可以说：使用 xx摄像头拍照并分析
5. 配置不同的聊天通道
    1. 按照使用说明，配置各个通道
