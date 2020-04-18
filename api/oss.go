package handler

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"gopkg.in/yaml.v2"
)

// Oss 阿里云Oss服务
type Oss struct {
	Ak       string `yaml:"Ak"`       //AccessKey ID
	Sk       string `yaml:"Sk"`       //Access Key Secret
	Bucket   string `yaml:"Bucket"`   //Bucket
	Endpoint string `yaml:"Endpoint"` //外网访问地域节点(非Bucket域名)
	Domain   string `yaml:"Domain"`   //自定义域名(Bucket域名或自定义)
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
	Oss     `yaml:"Oss"`
}

var (
	// OssConfig 配置信息
	OssConfig *Config
	// OssClient 操作仓库对象
	OssClient *oss.Bucket
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

// InitOssClient 初始化操作
func InitOssClient() *Response {
	OssConfig = GetConfig()
	client, err := oss.New(OssConfig.Endpoint, OssConfig.Ak, OssConfig.Sk)
	if err != nil {
		return &Response{
			Code:    500,
			Message: "ErrorInitClient:" + err.Error(),
		}
	}
	// 获取存储空间。
	OssClient, err = client.Bucket(OssConfig.Bucket)
	if err != nil {
		return &Response{
			Code:    500,
			Message: "ErrorInitBucket:" + err.Error(),
		}
	}
	return nil
}

// Handler 请求参数信息
// Operate: 操作类型 [list,delete,upload,domain,mkdir]
// Prefix: 操作的前缀(前缀意为操作所在的目录)
// Path: 操作的绝对地址

// OssHandler 句柄
func OssHandler(w http.ResponseWriter, r *http.Request) {
	//初始化
	if err := InitOssClient(); err != nil {
		response, _ = json.Marshal(err)
		Write(w, response)
		return
	}
	var operate = r.URL.Query().Get("operate")
	if operate == "list" {
		// 列举当前目录下的所有文件
		var result []ListObject //结果集
		//设置筛选器
		var path = r.URL.Query().Get("prefix")
		maker := oss.Marker(path)
		prefix := oss.Prefix(path)
		//结果入 result
		for {
			lsRes, err := OssClient.ListObjects(maker, prefix, oss.Delimiter("/"))
			if err != nil {
				response, _ = json.Marshal(&Response{
					Code:    500,
					Message: "ErrorListObject:" + err.Error(),
				})
				Write(w, response)
				return
			}
			for _, dirname := range lsRes.CommonPrefixes {
				result = append(result, ListObject{
					Filename:   strings.Replace(dirname, path, "", 1),
					CreateTime: time.Time{},
					IsDir:      true,
					Prefix:     path,
				})
			}
			for _, obj := range lsRes.Objects {
				result = append(result, ListObject{
					Filename:   strings.Replace(obj.Key, path, "", 1),
					CreateTime: obj.LastModified,
					IsDir:      false,
					Prefix:     path,
					Size:       obj.Size,
				})
			}
			prefix = oss.Prefix(lsRes.Prefix)
			maker = oss.Marker(lsRes.NextMarker)
			if !lsRes.IsTruncated {
				break
			}
		}
		response, _ = json.Marshal(&List{
			Code:    200,
			Message: OssConfig.Domain,
			Data:    result,
			Count:   len(result),
		})
	} else if operate == "delete" {
		//需要删除的文件绝对路径
		var path = r.URL.Query().Get("path")
		err := OssClient.DeleteObject(path)
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
		err = OssClient.PutObject(prefix+dst, source)
		if err != nil {
			response, _ = json.Marshal(&Response{
				Code:    500,
				Message: "ErrorObjectUpload:" + err.Error(),
			})
			Write(w, response)
			return
		}
		response, _ = json.Marshal(&Response{
			Code:    200,
			Message: "ok",
			Data:    OssConfig.Domain + prefix + dst,
		})
	} else if operate == "domain" {
		response, _ = json.Marshal(&Response{
			Code:    200,
			Message: OssConfig.Domain,
		})
	} else if operate == "mkdir" {
		var prefix = r.URL.Query().Get("prefix")
		var dirname = r.URL.Query().Get("dirname")
		err := OssClient.PutObject(prefix+dirname, nil)
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
	return
}
