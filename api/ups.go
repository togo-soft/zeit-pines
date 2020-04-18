package handler

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/upyun/go-sdk/upyun"
	"gopkg.in/yaml.v2"
)

// Ups 又拍云Ups服务
type Ups struct {
	Bucket   string `yaml:"Bucket"`   //服务名称
	Operator string `yaml:"Operator"` //授权的操作员名称
	Password string `yaml:"Password"` //授权的操作员密码
	Domain   string `yaml:"Domain"`   //加速域名
}

// Response 是交付层的基本回应
type Response struct {
	Code    int         `json:"code"`    //请求状态代码
	Message interface{} `json:"message"` //请求结果提示
	Data    interface{} `json:"data"`    //请求结果与错误原因
}

// List 会返回给交付层一个列表回应
type List struct {
	Code    int         `json:"code"`    //请求状态代码
	Count   int         `json:"count"`   //数据量
	Message interface{} `json:"message"` //请求结果提示
	Data    interface{} `json:"data"`    //请求结果
}

// ListObject 对象列表
type ListObject struct {
	Filename   string      `json:"filename"`
	Prefix     string      `json:"prefix"`
	IsDir      bool        `json:"is_dir"`
	Size       interface{} `json:"size"`
	CreateTime interface{} `json:"create_time"`
}

// Config 配置文件解析
type Config struct {
	Port    string `yaml:"Port"`
	Default string `yaml:"Default"`
	Token   string `yaml:"Token"`
	UToken  string `yaml:"UToken"`
	Ups     `yaml:"Ups"`
}

var (
	// UpsConfig 是又拍云配置项
	UpsConfig *Config
	//response 返回值
	response []byte
)

// GetConfig 调用该方法会实例化conf 项目运行会读取一次配置文件 确保不会有多余的读取损耗
func GetConfig() *Config {
	var config = new(Config)
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

// UpsHandler 逻辑处理
func UpsHandler(w http.ResponseWriter, r *http.Request) {
	//初始化
	UpsConfig = GetConfig()
	var up = upyun.NewUpYun(&upyun.UpYunConfig{
		Bucket:   UpsConfig.Bucket,
		Operator: UpsConfig.Operator,
		Password: UpsConfig.Password,
	})
	//执行何种操作
	var operate = r.URL.Query().Get("operate")
	if operate == "list" {
		var result []ListObject //结果集
		var prefix = r.URL.Query().Get("prefix") + "/"
		objsChan := make(chan *upyun.FileInfo, 10)
		go func() {
			up.List(&upyun.GetObjectsConfig{
				Path:        prefix,
				ObjectsChan: objsChan,
			})
		}()
		for obj := range objsChan {
			var filename string
			if obj.IsDir {
				filename = obj.Name + "/"
			} else {
				filename = obj.Name
			}
			result = append(result, ListObject{
				Filename:   filename,
				Prefix:     prefix,
				IsDir:      obj.IsDir,
				Size:       obj.Size,
				CreateTime: obj.Time,
			})
		}
		//返回信息
		response, _ = json.Marshal(&List{
			Code:    200,
			Message: UpsConfig.Domain,
			Data:    result,
			Count:   len(result),
		})
	} else if operate == "delete" {
		//需要删除的文件绝对路径
		var path = r.URL.Query().Get("path")
		//执行删除
		if err := up.Delete(&upyun.DeleteObjectConfig{
			Path:  path,
			Async: false,
		}); err != nil {
			//删除失败
			response, _ = json.Marshal(&Response{
				Code:    500,
				Message: "ErrorDelete:" + err.Error(),
			})
			Write(w, response)
			return
		}
		response, _ = json.Marshal(&Response{
			Code:    200,
			Message: "ok",
		})
	} else if operate == "upload" {
		var _, header, err = r.FormFile("file")
		var prefix string
		r.ParseMultipartForm(32 << 20)
		if r.MultipartForm != nil {
			values := r.MultipartForm.Value["prefix"]
			if len(values) > 0 {
				prefix = values[0]
			}
		}
		if err != nil {
			response, _ = json.Marshal(&Response{
				Code:    500,
				Message: "ErrorUpload:" + err.Error(),
			})
			Write(w, response)
			return
		}
		dst := header.Filename
		source, _ := header.Open()
		if err := up.Put(&upyun.PutObjectConfig{
			Path:   prefix + dst,
			Reader: source,
		}); err != nil {
			//上传失败
			response, _ = json.Marshal(&Response{
				Code:    500,
				Message: "ErrorUpload:" + err.Error(),
			})
			Write(w, response)
			return
		}
		response, _ = json.Marshal(&Response{
			Code:    200,
			Message: "ok",
			Data:    UpsConfig.Domain + prefix + dst,
		})
	} else if operate == "mkdir" {
		var prefix = r.URL.Query().Get("prefix")
		var dirname = r.URL.Query().Get("dirname")
		if err := up.Mkdir(prefix + dirname); err != nil {
			response, _ = json.Marshal(&Response{
				Code:    500,
				Message: "ErrorMkdir:" + err.Error(),
			})
			Write(w, response)
			return
		}
		response, _ = json.Marshal(&Response{
			Code:    200,
			Message: "ok",
		})
	} else if operate == "domain" {
		response, _ = json.Marshal(&Response{
			Code:    200,
			Message: UpsConfig.Domain,
		})
	}
	Write(w, response)
	return
}
