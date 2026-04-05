1. 前置准备
    1. 解压执行程序包
    2. 找到 ffmpeg https://ffmpeg.org/download.html、go2rtc https://github.com/AlexxIT/go2rtc/releases 对应的系统的可执行程序，放到 picoclaw-launcher 同目录下目录结构类似，以下以windows举例，其他系统根据不同系统文件没有exe后缀：
        ```
        D:\claw
        ├── ffmpeg.exe
        ├── go2rtc.exe
        ├── picoclaw-launcher.exe
        └── picoclaw.exe
        ```
    3. 执行picoclaw-launcher
    4. 会自动打开浏览器，访问http://localhost:18800
2. 设置登录令牌
    1. 第一次登录时，会弹出输入token，在桌面右下角找到picoclaw图标，右键copy dashboard token
    2. 粘贴到token输入的位置，登录进入
    3. 找到左侧菜单【服务】-【配置】页面，设置登录令牌，保存
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
    1. 访问http://localhost:18800 ，找到左侧菜单【智能家居】-【小米】
    2. 登录自己的小米账号
4. 同步&控制小米设备
    1. 在聊天框，输入同步小米设备，则homeclaw会自动化执行设备同步，如有多个家庭，则会提示选择一个家庭
    2. 设备同步完毕后，可以说：打开某个开关，关闭某个开关
    3. 如果有摄像头，可以说：使用 xx摄像头拍照并分析
6. 配置不同的聊天通道
    1. 按照使用说明，配置各个通道

视频链接
- [安装配置视频教程](https://www.bilibili.com/video/BV1uMDuBiEZQ/)