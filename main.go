package main

import (
	"archive/tar"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"io"
	"io/fs"
	"io/ioutil"
	"local_backups/common"
	"local_backups/uploader"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const ConfigKey = "localConfigPath"

const uploadFileKey = "filename"

const DefaultConfigValue = "./config.json"

const FlagUsage = "服务配置文件"

var globalWait sync.WaitGroup

var configLocalPath = flag.String(ConfigKey, DefaultConfigValue, FlagUsage)

var config = &common.Config{}

// 根据路径读取配置文件信息
func readLocalConfig(localConfigPath string) {
	// 判断文件是否存在
	if !common.IsFile(localConfigPath) {
		log.Panicf("Config file %s is not exist", localConfigPath)
	}
	buf, err := ioutil.ReadFile(localConfigPath)
	if err != nil {
		log.Panicf("Load config %s faild err: %s\n", configLocalPath, err)
	}
	err = json.Unmarshal(buf, config)
	if err != nil {
		log.Panicf("Decode config file %s failed err: %s\n", configLocalPath, err)
	}
}

func upload(w http.ResponseWriter, r *http.Request) {
	file, header, err := r.FormFile(uploadFileKey)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer file.Close()

	f, err := os.OpenFile(path.Join(config.ServerConfig.StoreDir, header.Filename), os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer f.Close()

	io.Copy(f, file)
}

func checkServerConfig(c common.Server) {
	if len(c.Address) == 0 {
		log.Panicf("Server config adress is nil")
	}

	if c.Port <= 0 {
		log.Panicf("Server port incorrect")
	}

	if len(c.StoreDir) == 0 {
		log.Panicf("Server config storeDir is nil")
	}
}

func tarFolder(fileSource string, traWriter *tar.Writer) error {
	return filepath.Walk(fileSource, func(targetpath string, info fs.FileInfo, err error) error {
		if info == nil {
			return err
		}
		if info.IsDir() {
			if fileSource == targetpath {
				return nil
			}
			header, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			header.Name = filepath.Join(fileSource, strings.TrimPrefix(targetpath, fileSource))
			if err = traWriter.WriteHeader(header); err != nil {
				return err
			}
			os.Mkdir(strings.TrimPrefix(fileSource, info.Name()), os.ModeDir)
			return tarFolder(targetpath, traWriter)
		}
		return err
	})
}

func start() {
	cn := cron.New()
	cn.AddFunc(config.ClientConfig.ClientTiming.Corn, func() {
		paths := config.ClientConfig.ClientTiming.LocalPaths
		for i := range paths {
			globalWait.Add(1)
			if common.IsDir(paths[i]) {
				tarPath := compress(paths[i])
				if len(tarPath) == 0 {
					fmt.Println("")
				} else {
					uploadFile(tarPath)
					os.RemoveAll(tarPath)
				}

			} else {
				uploadFile(paths[i])
			}
		}
	})
	cn.Start()
	select {}
}

func compress(path string) string {
	uuid, err := uuid.NewUUID()
	if err != nil {
		fmt.Println("uuid生成失败.")
		return ""
	}
	fileTarget := "./" + uuid.String() + filepath.Base(path) + ".tar"
	// 创建压缩文件
	tarFile, err := os.Create(fileTarget)

	if err != nil {
		if err == os.ErrExist {
			if err := os.Remove(fileTarget); err != nil {
				fmt.Println(err)
			}
		} else {
			fmt.Println(err)
		}
	}
	// 关闭文件
	defer tarFile.Close()
	// 写入文件句柄
	traWriter := tar.NewWriter(tarFile)
	// 获取文件信息
	fileInfo, err := os.Stat(path)
	if err != nil {
		fmt.Println(err)
	}
	// 判断是否是文件或者文件夹
	if fileInfo.IsDir() {
		tarFolder(path, traWriter)
	}
	return fileTarget
}

func uploadFile(uploadFilePath string) {
	defer globalWait.Done()

	var err error = nil
	// 获取文件大小
	filesize := common.GetFileSize(uploadFilePath)
	// 是否需要进行分片上传
	if filesize <= common.SmallFileSize {
		err = uploader.UploadFile(uploadFilePath)
	} else {
		// 大文件进行切片
		localUpload := uploader.NewUploader(uploadFilePath, common.SliceBytes)
		if localUpload == nil {
			localUpload = uploader.NewUploader(uploadFilePath, common.SliceBytes)
		}
		if localUpload == nil {
			fmt.Println("创建上传器失败，上传文件失败, " + uploadFilePath)
			return
		}
		err = localUpload.UploadFileBySlice()
	}

	if err != nil {
		fmt.Printf("上传%s文件失败\n", uploadFilePath)
	}
}

func createUploadDir(w http.ResponseWriter, r *http.Request) {
	var cMetadata common.ClientFileMetadata
	err := json.NewDecoder(r.Body).Decode(&cMetadata)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	// 检查文件是否已存在
	metadataPath := common.GetMetadataFilepath(path.Join(config.ServerConfig.StoreDir, cMetadata.Filename))
	if common.IsFile(metadataPath) {
		http.Error(w, "文件已存在", http.StatusBadRequest)
		return
	}

	uploadDir := path.Join(config.ServerConfig.StoreDir, cMetadata.Fid)
	err = os.Mkdir(uploadDir, 0766)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	sMetadata := common.ServerFileMetadata{
		ClientFileMetadata: cMetadata,
		State:              "uploading",
	}

	//写元数据文件
	err = common.StoreMetadata(metadataPath, &sMetadata)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
}

// 接收分片文件函数
func receiveSliceFile(w http.ResponseWriter, r *http.Request) {
	var part common.FilePart
	err := json.NewDecoder(r.Body).Decode(&part)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	sliceFilename := path.Join(config.ServerConfig.StoreDir, part.Fid, strconv.Itoa(part.Index))
	if common.IsFile(sliceFilename) {
		fmt.Printf("%s分片文件已存在，直接丢弃, part.Fid: %s, index: %s\n", sliceFilename, part.Fid, strconv.Itoa(part.Index))
	}

	err = ioutil.WriteFile(sliceFilename, part.Data, 0666)
	if err != nil {
		fmt.Println(err)
		return
	}
}

func mergeSliceFiles(w http.ResponseWriter, r *http.Request) {
	// 不真正进行合并，只计算md5进行数据准确性校验

	var cMetadata common.ClientFileMetadata
	err := json.NewDecoder(r.Body).Decode(&cMetadata)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	uploadDir := path.Join(config.ServerConfig.StoreDir, cMetadata.Fid)
	hash := md5.New()
	log.Printf("开始合并：%s \n", cMetadata.Fid)
	// 计算md5
	for i := 0; i < cMetadata.SliceNum; i++ {
		sliceFilePath := path.Join(uploadDir, strconv.Itoa(i))
		sliceFile, err := os.Open(sliceFilePath)
		if err != nil {
			fmt.Printf("读取文件%s失败, err: %s\n", sliceFilePath, err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		io.Copy(hash, sliceFile)
		sliceFile.Close()
	}

	md5Sum := hex.EncodeToString(hash.Sum(nil))
	if md5Sum != cMetadata.Md5sum {
		fmt.Println("文件md5校验不通过，数据传输有误，请重新上传文件！")
		fmt.Printf("md5校验失败，原始md5：%s, 计算的md5：%s\n", cMetadata.Md5sum, md5Sum)
		http.Error(w, err.Error(), http.StatusBadRequest)
		// 删除保存文件夹
		os.RemoveAll(uploadDir)
		return
	}

	fmt.Printf("md5校验成功，原始md5：%s, 计算的md5：%s\n", cMetadata.Md5sum, md5Sum)

	// 更新元数据信息
	metadataPath := common.GetMetadataFilepath(path.Join(config.ServerConfig.StoreDir, cMetadata.Filename))
	metadata, err := common.LoadMetadata(metadataPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	metadata.Md5sum = md5Sum
	metadata.State = "active"
	err = common.StoreMetadata(metadataPath, metadata)
	if err != nil {
		fmt.Println("更新元数据文件失败，上传失败")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = MergeFile(metadata)
	if err != nil{
		fmt.Println("文件合并失败")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	os.RemoveAll(metadataPath)
	os.RemoveAll(path.Join(config.ServerConfig.StoreDir,metadata.Fid))
}

func MergeFile(serverFileMetadata *common.ServerFileMetadata) error {
	clientMetadata := serverFileMetadata.ClientFileMetadata
	fmt.Println("开始合并文件", clientMetadata.Filename)
	targetFile := path.Join(config.ServerConfig.StoreDir, serverFileMetadata.Filename)
	f, err := os.OpenFile(targetFile, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		fmt.Println(err)
		return err
	}

	md5hash := md5.New()

	sliceDir := path.Join(config.ServerConfig.StoreDir, serverFileMetadata.Fid)
	for i := 0; i < clientMetadata.SliceNum; i++ {
		sliceFilePath := path.Join(sliceDir, strconv.Itoa(i))
		sliceFile, err := os.Open(sliceFilePath)
		if err != nil {
			fmt.Printf("读取文件%s失败, err: %s\n", sliceFilePath, err)
			return err
		}
		io.Copy(md5hash, sliceFile)

		// 偏移量需要重新进行调整
		sliceFile.Seek(0, 0)
		io.Copy(f, sliceFile)

		// 偏移量需要重新进行调整
		sliceFile.Seek(0, 0)
		io.Copy(f, sliceFile)

		sliceFile.Close()
	}
	// 校验md5值
	calMd5 := hex.EncodeToString(md5hash.Sum(nil))
	if calMd5 != clientMetadata.Md5sum {
		fmt.Printf("%s文件校验失败，请重新下载, 原始md5: %s, 计算的md5: %s\n", clientMetadata.Filename, clientMetadata.Md5sum, calMd5)
		return errors.New("文件校验失败")
	}
	f.Close()
	fmt.Printf("%s文件合并成功，保存路径：%s\n", clientMetadata.Filename, targetFile)
	return nil
}

func main() {
	flag.Parse()

	// 读取配置文件
	readLocalConfig(*configLocalPath)

	// 进行配置校验
	server := config.ServerConfig
	checkServerConfig(server)

	client := config.ClientConfig

	if len(client.ServerIp) != 0 && client.ServerPort > 0 {
		common.BaseUrl = fmt.Sprintf("http://%s:%d/", client.ServerIp, client.ServerPort)
		time := client.ClientTiming
		if time.Enabled && len(time.Corn) > 0 && len(time.LocalPaths) > 0 {
			go start()
		}
	}

	// 进行端口绑定
	http.HandleFunc("/upload", upload)
	http.HandleFunc("/startUploadSlice", createUploadDir)
	http.HandleFunc("/uploadBySlice", receiveSliceFile)
	http.HandleFunc("/mergeSlice", mergeSliceFiles)

	err := http.ListenAndServe(":"+strconv.Itoa(server.Port), nil)
	if err != nil {
		log.Fatal("ListenAndServer:  ", err)
	}
}
