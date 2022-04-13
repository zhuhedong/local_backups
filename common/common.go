package common

import "time"

// SmallFileSize 定义常量
const SmallFileSize = 1024 * 1024 * 100  // 小文件大小
const SliceBytes = 1024 * 1024 * 100  // 分片大小
const UploadRetryChannelNum = 100   // 上传的重试通道队列大小
const DownloadRetryChannelNum = 100 // 下载的重试通道队列大小
const UploadTimeout = 300           // 上传超时时间，单位秒
const DownloadTimeout = 300         // 上传超时时间，单位秒
const UpGoroutineMaxNumPerFile = 10 // 每个上传文件开启的goroutine最大数量
const DpGoroutineMaxNumPerFile = 10 // 每个下载文件开启的goroutine最大数量

// BaseUrl 定义公共变量
var BaseUrl string

type Config struct {
	ClientConfig Client `json:"clientConfig"`
	ServerConfig Server `json:"serverConfig"`
}

// Client 客户端配置信息
type Client struct {
	ServerIp     string `json:"serverIp"`
	ServerPort   int    `json:"serverPort"`
	ClientTiming Timing `json:"clientTiming"`
}

// Timing 定时器信息
type Timing struct {
	Enabled    bool     `json:"enabled"`    //是否开启定时上传
	Corn       string   `json:"cron"`       //定时任务表达式,
	LocalPaths []string `json:"localPaths"` //  本地文件监控地址
}

// Server 服务端配置信息
type Server struct {
	Port     int    `json:"port"`     // 服务端口
	Address  string `json:"address"`  // 服务绑定地址
	StoreDir string `json:"storeDir"` // 文件上传后存储文件夹
}

// FilePart 文件分片结构
type FilePart struct {
	Fid   string // 操作文件ID，随机生成的UUID
	Index int    // 文件切片序号
	Data  []byte // 分片数据
}

// ClientFileMetadata 客户端传来的文件元数据结构
type ClientFileMetadata struct {
	Fid        string    // 操作文件ID，随机生成的UUID
	Filesize   int64     // 文件大小（字节单位）
	Filename   string    // 文件名称
	SliceNum   int       // 切片数量
	Md5sum     string    // 文件md5值
	ModifyTime time.Time // 文件修改时间
}

// ServerFileMetadata 服务端保存的文件元数据结构
type ServerFileMetadata struct {
	ClientFileMetadata        // 隐式嵌套
	State              string // 文件状态，目前有uploading、downloading和active
}

type SliceSeq struct {
	Slices []int // 需要重传的分片号
}

// FileInfo 文件列表单元结构
type FileInfo struct {
	Filename string // 文件名
	Filesize int64  // 文件大小
	Filetype string // 文件类型（目前有普通文件和切片文件两种）
}

// ListFileInfos 文件列表结构
type ListFileInfos struct {
	Files []FileInfo
}


// FileMetadata 文件片元数据
type FileMetadata struct {
	Fid             string          // 操作文件ID，随机生成的UUID
	Filesize        int64           // 文件大小（字节单位）
	Filename        string          // 文件名称
	SliceNum        int             // 切片数量
	Md5sum          string          // 文件md5值
	ModifyTime      time.Time       // 文件修改时间
}

