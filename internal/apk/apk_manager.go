// Package apk 定义嵌入的 scrcpy-server 路径、版本号等常量。
package apk

// 官方 scrcpy-server（jar），由系统嵌入并推送后 CLASSPATH 启动 com.genymobile.scrcpy.Server。
// ScrcpyServerClientVersion 必须与 jar 内 BuildConfig.VERSION_NAME 一致。
const (
	ScrcpyServerClientVersion = "3.3.3"
	ScrcpyServerEmbedPath     = "assets/apks/scrcpy-server@" + ScrcpyServerClientVersion
	ScrcpyServerRemotePath    = "/data/local/tmp/scrcpy-server.jar"
)
