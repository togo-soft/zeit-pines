package handler

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"

	"gopkg.in/yaml.v2"
)

// Config 配置文件解析
type Config struct {
	Port    string `yaml:"Port"`
	Default string `yaml:"Default"`
	Token   string `yaml:"Token"`
	UToken  string `yaml:"UToken"`
}

var (
	//response 返回值
	response []byte
	// config 是一个全局的配置信息实例 项目运行只读取一次 是一个单例
	config *Config
)

// GetConfig 调用该方法会实例化conf 项目运行会读取一次配置文件 确保不会有多余的读取损耗
func GetConfig() *Config {
	config = new(Config)
	yamlFile, err := ioutil.ReadFile("config.yaml")
	if err != nil {
		panic(err)
	}
	err = yaml.Unmarshal(yamlFile, config)
	if err != nil {
		//读取配置文件失败,停止执行
		panic("read config file error:" + err.Error())
	}
	return config
}

// Write 输出返回结果
func Write(w http.ResponseWriter, response []byte) {
	//公共的响应头设置
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, OPTIONS")
	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(string(response))))
	_, _ = w.Write(response)
	return
}

// GetUploadAPI 获取快捷上传接口
func GetUploadAPI(w http.ResponseWriter, r *http.Request) {
	GetConfig()
	response, _ = json.Marshal(struct {
		Code   int         `json:"code"`
		Utoken string      `json:"utoken"`
		URL    interface{} `json:"url"`
	}{
		Code:   200,
		Utoken: config.UToken,
		URL:    config.Default,
	})
	Write(w, response)
	return
}
