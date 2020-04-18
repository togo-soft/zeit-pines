package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"io/ioutil"
	"strconv"

	"github.com/tencentyun/cos-go-sdk-v5"
	"gopkg.in/yaml.v2"
)

// Cos 腾讯云Cos服务
type Cos struct {
	SecretID   string `yaml:"SecretID"`   //API密钥ID
	SecretKey  string `yaml:"SecretKey"`  //API密钥私钥
	Bucket     string `yaml:"Bucket"`     //存储桶名称 规则 test-1234567889
	Region     string `yaml:"Region"`     //存储桶所属地域 规则 ap-nanjing
	Domain     string `yaml:"Domain"`     //自定义域名
	APIAddress string `yaml:"APIAddress"` //API地址(访问域名) 在存储桶列表->配置管理->基础配置中可见 规则 https://<bucket>.cos.<region>.myqcloud.com
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
	Cos     `yaml:"Cos"`
}

var (
	// CosConfig 配置项
	CosConfig *Config
	// CosClient 客户端
	CosClient *cos.Client
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

// InitCosClient 初始化操作
func InitCosClient() {
	CosConfig = GetConfig()
	CosConfig.APIAddress = fmt.Sprintf("https://%s.cos.%s.myqcloud.com", CosConfig.Bucket, CosConfig.Region)
	u, _ := url.Parse(CosConfig.APIAddress)
	b := &cos.BaseURL{BucketURL: u}
	CosClient = cos.NewClient(b, &http.Client{
		//设置超时时间
		Timeout: 100 * time.Second,
		Transport: &cos.AuthorizationTransport{
			//如实填写账号和密钥，也可以设置为环境变量
			SecretID:  CosConfig.SecretID,
			SecretKey: CosConfig.SecretKey,
		},
	})
}

// Handler 请求参数信息
// Operate: 操作类型 [list,delete,upload,domain,mkdir]
// Prefix: 操作的前缀(前缀意为操作所在的目录)
// Path: 操作的绝对地址

// CosHandler 句柄
func CosHandler(w http.ResponseWriter, r *http.Request) {
	//初始化
	InitCosClient()
	var operate = r.URL.Query().Get("operate")
	if operate == "list" {
		// 列举当前目录下的所有文件
		var result []ListObject //结果集
		//设置筛选器
		var prefix = r.URL.Query().Get("prefix")
		opt := &cos.BucketGetOptions{
			Prefix:    prefix,
			Delimiter: "/",
			Marker:    prefix,
		}
		//结果入 result
		v, _, err := CosClient.Bucket.Get(context.Background(), opt)
		if err != nil {
			response, _ = json.Marshal(&Response{
				Code:    500,
				Message: "ErrorListObject:" + err.Error(),
			})
			Write(w, response)
			return
		}
		for _, dirname := range v.CommonPrefixes {
			result = append(result, ListObject{
				Filename:   strings.Replace(dirname, prefix, "", 1),
				CreateTime: "",
				IsDir:      true,
				Prefix:     prefix,
			})
		}
		for _, obj := range v.Contents {
			result = append(result, ListObject{
				Filename:   strings.Replace(obj.Key, prefix, "", 1),
				CreateTime: obj.LastModified,
				IsDir:      false,
				Prefix:     prefix,
				Size:       obj.Size,
			})
		}

		var domain string
		if CosConfig.Domain == "" {
			domain = CosConfig.APIAddress + "/"
		} else {
			domain = CosConfig.Domain
		}
		response, _ = json.Marshal(&List{
			Code:    200,
			Message: domain,
			Data:    result,
			Count:   len(result),
		})
	} else if operate == "delete" {
		//需要删除的文件绝对路径
		var path = r.URL.Query().Get("path")
		_, err := CosClient.Object.Delete(context.Background(), path)
		if err != nil {
			response, _ = json.Marshal(&Response{
				Code:    500,
				Message: "ErrorObjectDelete:" + err.Error(),
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
		_ = r.ParseMultipartForm(32 << 20)
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
		_, err = CosClient.Object.Put(context.Background(), prefix+dst, source, nil)
		if err != nil {
			response, _ = json.Marshal(&Response{
				Code:    500,
				Message: "ErrorObjectUpload:" + err.Error(),
			})
			Write(w, response)
			return
		}
		var domain string
		if CosConfig.Domain == "" {
			domain = CosConfig.APIAddress + "/"
		} else {
			domain = CosConfig.Domain
		}
		response, _ = json.Marshal(&Response{
			Code:    200,
			Message: "ok",
			Data:    domain + prefix + dst,
		})
	} else if operate == "domain" {
		var domain string
		if CosConfig.Domain == "" {
			domain = CosConfig.APIAddress + "/"
		} else {
			domain = CosConfig.Domain
		}
		response, _ = json.Marshal(&Response{
			Code:    200,
			Message: domain,
		})
	} else if operate == "mkdir" {
		var prefix = r.URL.Query().Get("prefix")
		var dirname = r.URL.Query().Get("dirname")
		_, err := CosClient.Object.Put(context.Background(), prefix+dirname, nil, nil)
		if err != nil {
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
	}
	Write(w, response)
}
