package controllers

import (
	"TiCheck/insight/controllers/handler"

	"github.com/gin-gonic/gin"
)

func Register(engine *gin.Engine) {

	engine.Static("/assets", "./views/assets")
	engine.LoadHTMLGlob("./views/*.html")

	viewGroup := engine.Group("/")
	{
		view := &handler.ViewHandler{}

		// 打开首页
		viewGroup.GET("/", view.GetIndex)

		// 打开登录界面
		viewGroup.GET("/login", view.GetLogin)
	}

	sessionGroup := engine.Group("/session")
	session := &handler.SessionHandler{
		Sessions: make(map[string]*handler.Session, 0),
	}

	{
		// 用户认证
		sessionGroup.POST("/", session.AuthenticatedUser)

		// 退出用户
		sessionGroup.POST("/logout", session.Logout)
	}

	reportGroup := engine.Group("/report")
	reportGroup.Use(session.VerifyToken)
	{
		report := &handler.ReportHandler{}

		// 获取历史巡检列表
		reportGroup.GET("/catalog", report.GetCatalog)

		// 通过id获得某次巡检结果
		reportGroup.GET("/id/:id", report.GetReport)

		// 获取最后一次巡检结果
		reportGroup.GET("/last", report.GetLastReport)

		// 获取巡检结果元信息
		reportGroup.GET("/meta", report.GetMeta)

		// 执行一次巡检
		reportGroup.GET("/", report.ExecuteCheck)

		// 下载所有的巡检报告
		reportGroup.GET("/download/all", report.DownloadAllReport)

		// 下载指定的一次巡检报告
		reportGroup.GET("/download/:id", report.DownloadReport)

		// 编辑配置脚本
		reportGroup.POST("/editconf/:script", report.EditConfig)
	}

	scriptGroup := engine.Group("/script")
	// test, ignore token
	// scriptGroup.Use(session.VerifyToken)
	{
		script := &handler.ScriptHandler{}

		// 查看所有本地脚本
		scriptGroup.GET("/local", script.GetAllLocalScript)

		// 查看所有的远程仓库脚本，获取列表
		scriptGroup.GET("/remote", script.GetAllRemoteScript)

		// 查看指定远程脚本的介绍
		scriptGroup.GET("/remote/readme/:name", script.GetReadMe)

		// 下载指定名的脚本到本地
		scriptGroup.POST("/remote/download/:name", script.DownloadScript)
	}
}
