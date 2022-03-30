package handler

import (
	"TiCheck/cmd/ticheck-server/api"
	"TiCheck/internal/model"
	"TiCheck/internal/util"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	custom_prefix = "../../probes/custom/"
)

type StoreHandler struct {
}

type RemoteScriptList struct {
	Total   int             `json:"total"`
	Scripts []*RemoteScript `json:"script_list"`
}

type RemoteScript struct {
	Name     string `json:"name"`
	Download bool   `json:"download"`
}

type LocalScriptList struct {
	Total   int            `json:"total"`
	Scripts []*LocalScript `json:"script_list"`
}

type LocalScript struct {
	Name string `json:"name"`
}

// GetLocalScript 获取自定义所有脚本列表
func (s *StoreHandler) GetCustomScript(c *gin.Context) {

	pg := &model.Paginator{}

	pg.AddFilter("is_system = ?", 0)

	s.GetScriptListInDB(c, pg)
}

func (s *StoreHandler) GetCustomReadme(c *gin.Context) {
	id := c.Param("id")
	s.getreadme(c, "custom", id)
}

func (s *StoreHandler) DeleteCustomReadme(c *gin.Context) {
	id := c.Param("id")
	p := model.Probe{
		ID: id,
	}
	if err := p.GetByID(); err != nil || p.IsSystem != 0 {
		api.BadWithMsg(c, "probe not found: "+id)
		return
	}
	if p.Delete() != nil {
		api.BadWithMsg(c, "probe can not be deleted")
		return
	}
	os.RemoveAll(fmt.Sprintf("%s%s", custom_prefix, id))
	api.Success(c, "probe deleted", nil)
}

func (s *StoreHandler) UploadCustomScript(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		api.BadWithMsg(c, "file not found: "+err.Error())
		return
	}

	dst := fmt.Sprintf("%s%s", custom_prefix, file.Filename)

	if err := c.SaveUploadedFile(file, dst); err != nil {
		api.Fail(c, "failed to upload: "+err.Error(), nil)
		return
	}
	defer os.Remove(dst)

	pkg := file.Filename[:len(file.Filename)-4]
	dir := fmt.Sprintf("%s%s", custom_prefix, pkg)
	if err := util.DeCompress(dst, dir); err != nil {
		api.FailWithMsg(c, "failed to decompress: "+err.Error())
		return
	}

	// resolver metadata file
	packageJSON := fmt.Sprintf("%s/package.json", dir)
	bytes, err := ioutil.ReadFile(packageJSON)
	if err != nil {
		defer os.RemoveAll(dir)
		api.FailWithMsg(c, "failed to read package.json: "+err.Error())
		return
	}
	pm := &model.ProbeMeta{}
	if err = json.Unmarshal(bytes, pm); err != nil || pkg != pm.ID {
		defer os.RemoveAll(dir)
		api.FailWithMsg(c, "failed to parse package.json: ")
		return
	}
	// verify probe ID
	p := model.Probe{
		ID: pm.ID,
	}
	if !p.IsNotExist() {
		defer os.RemoveAll(dir)
		api.FailWithMsg(c, "failed to upload: ID already exists")
		return
	}
	p.ScriptName = pm.Name
	p.FileName = pm.Main
	p.Description = pm.Description
	p.IsSystem = 0
	p.Creator = pm.Author.Name
	p.CreateTime = time.Time(pm.CreateTime).Local()
	p.UpdateTime = time.Time(pm.UpdateTime).Local()
	if len(pm.Tags) > 0 {
		t, ok := model.Dict_ProbeTags[pm.Tags[0]]
		if ok {
			p.Tag = t
		} else {
			p.Tag = "others"
		}
	}
	if len(pm.Rules) > 0 {
		p.Comparator.Operator = pm.Rules[0].Operator
		p.Comparator.Threshold = pm.Rules[0].Threshold
		p.Comparator.Arg = strings.Join(pm.Rules[0].Args, " ")
	}
	if err := p.Create(); err != nil {
		defer os.RemoveAll(dir)
		api.FailWithMsg(c, "failed to create probe: "+err.Error())
		return
	}
	api.Success(c, fmt.Sprintf("'%s' uploaded!", file.Filename), nil)
}

// GetLocalScript 获取本地所有脚本列表
func (s *StoreHandler) GetLocalScript(c *gin.Context) {

	pg := &model.Paginator{}

	pg.AddFilter("is_system = ?", 1)

	s.GetScriptListInDB(c, pg)
}

func (s *StoreHandler) GetLocalReadme(c *gin.Context) {
	name := c.Query("name")
	s.getreadme(c, "local", name)
}

func (s *StoreHandler) getreadme(c *gin.Context, path, name string) {
	filePath := fmt.Sprintf("../../probes/%s/%s/readme.md", path, name)
	_, err := os.Stat(filePath)
	if err != nil || os.IsNotExist(err) {
		api.BadWithMsg(c, "failed to get readme: "+err.Error())
		return
	}
	data, _ := ioutil.ReadFile(filePath)
	api.Success(c, "success", string(data))
}

func (s *StoreHandler) GetScriptListInDB(c *gin.Context, pg *model.Paginator) {

	tag := c.Query("tag")
	name := c.Query("name")

	p := &model.Probe{}

	if tag != "" && tag != "all" {
		pg.AddFilter("tag = ?", tag)
	}
	if name != "" {
		pg.AddFilter("script_name like ?", "%"+name+"%")
	}

	p.GetPager(c, pg)

	if pg.Err != nil {
		api.BadWithMsg(c, "failed to get list: "+pg.Err.Error())
		return
	}
	api.Success(c, "success", gin.H{
		"total": pg.Total,
		"rows":  pg.Rows,
	})
}

// GetAllRemoteScript 获取远程仓库脚本列表
func (s *StoreHandler) GetAllRemoteScript(c *gin.Context) {
	start := c.Query("start")
	length := c.Query("length")

	st, err := strconv.Atoi(start)
	if err != nil {
		st = 0
	}

	le, err := strconv.Atoi(length)
	if err != nil && le == 0 {
		le = 10
	}

	url := "https://api.github.com/repos/DigitalChinaOpenSource/TiCheck_ScriptWarehouse/contents/scripts"

	//remoteList := make([]string, 0)
	localList := make([]string, 0)

	scriptList := &RemoteScriptList{}

	// files, err := ioutil.ReadDir("../script/")
	// if err != nil {
	// 	c.JSON(http.StatusBadRequest, gin.H{
	// 		"error": err.Error(),
	// 	})
	// 	return
	// }

	// for _, f := range files {
	// 	name := strings.Split(f.Name(), ".")
	// 	localList = append(localList, name[0])
	// }

	jsonMap, err := s.SendRequest(url)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	for i, _ := range jsonMap {
		data, ok := jsonMap[i]["name"].(string)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "can't find script for remote warehouse, please check whether the remote warehouse is valid : " + url,
			})
			return
		}

		script := &RemoteScript{}
		//remoteList = append(remoteList, data)
		isDownload := false
		for _, v := range localList {
			if data == v {
				isDownload = true
				break
			}
		}

		script.Name = data
		script.Download = isDownload

		scriptList.Total += 1
		scriptList.Scripts = append(scriptList.Scripts, script)
	}

	if st+le > len(scriptList.Scripts) {
		le = len(scriptList.Scripts) - st
	}

	if st > len(scriptList.Scripts) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "exceeded the maximum number of scripts",
		})
		return
	}

	scriptList.Scripts = scriptList.Scripts[st : le+st]

	c.JSON(http.StatusOK, scriptList)
	return
}

// GetReadMe 获取远程仓库某个脚本的 Readme 文件并返回
func (s *StoreHandler) GetReadMe(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "script name not specified",
		})
		return
	}

	url := "https://raw.githubusercontent.com/DigitalChinaOpenSource/TiCheck_ScriptWarehouse/main/scripts/" + name + "/readme.md"
	//url1 := "https://raw.githubusercontent.com/DigitalChinaOpenSource/TiCheck_ScriptWarehouse/main/scripts/" + name + "/"+ name + ".config"

	resp, err := http.Get(url)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "failed to access remote warehouse: " + err.Error(),
		})
		return
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "failed to get readme: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"readme": string(body),
	})

	return
}

// DownloadScript 下载远程仓库脚本到本地
func (s *StoreHandler) DownloadScript(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "script name not specified",
		})
		return
	}

	isExist, err := s.CheckScriptIsExist(name)
	if isExist {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "the script already exists locally",
		})
		return
	}

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	scriptFileUrl := "https://api.github.com/repos/DigitalChinaOpenSource/TiCheck_ScriptWarehouse/contents/scripts/" + name

	// 由于可能存在不同的脚本语言不同，脚本名的后缀可能是 .py 或者 .sh, 需要获取脚本名的全文（包括后缀）
	// 但规定该文件下只有三个文件，分表是 script_name.py || script_name.sh, script_name.config 和 readme.md

	jsonMap, err := s.SendRequest(scriptFileUrl)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	var fileName string

	for i, _ := range jsonMap {
		value, ok := jsonMap[i]["name"].(string)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "can't find script for remote warehouse, please check whether the remote warehouse is valid : " + scriptFileUrl,
			})
			return
		}

		if value != name+".config" && value != "readme.md" {
			fileName = value
			break
		} else {
			if i == len(jsonMap)-1 {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "can't find script for remote warehouse, please check whether the remote warehouse is valid : " + scriptFileUrl,
				})
				return
			}
		}
	}

	configUrl := "https://raw.githubusercontent.com/DigitalChinaOpenSource/TiCheck_ScriptWarehouse/main/scripts/" + name + "/" + name + ".config"

	scriptUrl := "https://raw.githubusercontent.com/DigitalChinaOpenSource/TiCheck_ScriptWarehouse/main/scripts/" + name + "/" + fileName

	if err := s.saveScript(scriptUrl, fileName); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})

		return
	}

	if err := s.updateConfig(configUrl); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})

		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
	return
}

func (s *StoreHandler) SendRequest(url string) ([]map[string]interface{}, error) {
	resp, err := http.Get(url)

	if err != nil {
		return nil, err
	}

	body, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		return nil, err
	}

	resp.Body.Close()

	jsonMap := make([]map[string]interface{}, 0)

	json.Unmarshal(body, &jsonMap)

	return jsonMap, nil
}

// CheckScriptIsExist 检查改脚本本地是否已经存在
func (s *StoreHandler) CheckScriptIsExist(name string) (bool, error) {
	files, err := ioutil.ReadDir("../script/")
	if err != nil {
		return false, err
	}

	for _, f := range files {
		localName := strings.Split(f.Name(), ".")
		if localName[0] == name {
			return true, nil
		}
	}

	return false, nil
}

// saveSctipt 通过 url 下载脚本文件保存到本地
// 本地文件保存目录： ../script/
func (s *StoreHandler) saveScript(scriptUrl string, scriptName string) error {
	resp, err := http.Get(scriptUrl)

	if err != nil {
		return err
	}

	out, err := os.Create("../script/" + scriptName)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

// updateConfig 更新配置文件
// 配置文件目录：
func (s *StoreHandler) updateConfig(configUrl string) error {
	resp, err := http.Get(configUrl)

	if err != nil {
		return err
	}

	f, err := os.OpenFile("../config/execution_config.csv", os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	// 查找文件末尾的偏移量
	n, _ := f.Seek(0, os.SEEK_END)
	// 从末尾的偏移量开始写入内容
	_, err = f.WriteAt(body, n)

	defer f.Close()

	if err != nil {
		return err
	}

	return nil
}
