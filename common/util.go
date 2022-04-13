package common

import (
	"encoding/gob"
	"errors"
	"fmt"
	"os"
	"path"
)

// IsDir 判断path 是否为文件夹
func IsDir(path string) bool  {
	s, err := os.Stat(path)
	if err != nil {
		fmt.Printf("读取文件异常。%s\n", err.Error())
		return false
	}
	return s.IsDir()
}

// IsFile 判断文件是否存在
func IsFile(path string) bool {
	s, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !s.IsDir()
}

// GetMetadataFilepath 获取文件元数据文件路径
func GetMetadataFilepath(filePath string) string {
	return filePath + ".slice"
}


// LoadMetadata 加载元数据文件信息
func LoadMetadata(filePath string) (*ServerFileMetadata, error) {
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Println("获取文件状态失败，文件路径：", filePath)
		return nil, err
	}

	var metadata ServerFileMetadata
	fileData := gob.NewDecoder(file)
	err = fileData.Decode(&metadata)
	if err != nil {
		fmt.Println("格式化文件元数据失败, err", err)
		return nil, err
	}
	return &metadata, nil
}


// StoreMetadata 写元数据文件信息
func StoreMetadata(filePath string, metadata *ServerFileMetadata) error {
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		fmt.Println("创建元数据文件失败")
		return err
	}

	enc := gob.NewEncoder(f)
	err = enc.Encode(metadata)
	if err != nil {
		fmt.Println("写元数据文件失败")
		return err
	}

	f.Close()
	return nil
}

func ClientStoreMetadata(filePath string, metadata *FileMetadata) (error) {
	// 写入文件
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		fmt.Printf("写元数据文件%s失败\n", filePath)
		return err
	}
	defer file.Close()

	enc := gob.NewEncoder(file)
	err = enc.Encode(metadata)
	if err != nil {
		fmt.Printf("写元数据文件%s失败\n", filePath)
		return err
	}
	return nil
}


func CheckFileExist(fid string, filename string, storeDir string) (bool, error) {
	metadataPath := GetMetadataFilepath(path.Join(storeDir, filename))

	// 校验fid和filename是匹配的
	metadata, err := LoadMetadata(metadataPath)
	if err != nil {
		return false, err
	}
	if metadata.Fid != fid {
		fmt.Println("文件名和fid对不上，请确认后重试")
		return false, errors.New("文件名和fid对不上，请确认后重试")
	}

	return true, nil
}

// GetFileSize 获取文件大小
func GetFileSize(path string) int64 {
	fh, err := os.Stat(path)
	if err != nil {
		fmt.Printf("读取文件%s失败, err: %s\n", path, err)
	}
	return fh.Size()
}
