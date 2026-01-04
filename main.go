package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// 为不同类型的日志定义数据结构
type CommittedState struct {
	Timestamp time.Time `bson:"timestamp"`
	Module    string    `bson:"module"`
	Height    int64     `bson:"height"`
	Txs       int       `bson:"txs"`
	AppHash   string    `bson:"appHash"`
}

func main() {
	// 从环境变量获取配置
	mongoDBURI := getEnv("MONGO_URI", "mongodb://localhost:27017")
	mongoDBDatabase := getEnv("MONGO_DATABASE", "node_logs")
	logDir := getEnv("LOG_DIR", "./logs") // 从环境变量获取日志目录
	mainLogName := getEnv("MAIN_LOG_NAME", "stdout-xx.txt")

	log.Printf("数据库URI: %s", mongoDBURI)
	log.Printf("数据库名: %s", mongoDBDatabase)
	log.Printf("日志目录: %s", logDir)

	// 连接到 MongoDB
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(mongoDBURI))
	if err != nil {
		log.Fatalf("无法连接到 MongoDB: %v", err)
	}
	defer client.Disconnect(context.Background())
	err = client.Ping(context.Background(), nil)
	if err != nil {
		log.Fatalf("无法 Ping通 MongoDB: %v", err)
	}
	log.Println("成功连接到 MongoDB!")
	db := client.Database(mongoDBDatabase)

	// 1. 处理历史日志文件
	processHistoricalLogs(logDir, mainLogName, db)

	// 2. 处理当前的主日志文件
	mainLogFile := filepath.Join(logDir, mainLogName)
	processSingleFile(mainLogFile, db)

	// 3. 实时监听主日志文件
	watchLogFile(mainLogFile, db)
}

// getEnv 获取环境变量，如果不存在则返回默认值
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// processHistoricalLogs 查找并按顺序处理历史日志文件
func processHistoricalLogs(logDir string, mainLogName string, db *mongo.Database) {
	log.Println("开始处理历史日志文件...")
	files, err := os.ReadDir(logDir)
	if err != nil {
		log.Printf("无法读取日志目录 '%s': %v", logDir, err)
		return
	}

	re := regexp.MustCompile(fmt.Sprintf(`^%s\.(\d+)$`, mainLogName))
	var historicalLogs []string

	for _, file := range files {
		if !file.IsDir() && re.MatchString(file.Name()) {
			historicalLogs = append(historicalLogs, file.Name())
		}
	}

	// 按文件名中的数字后缀排序
	sort.Slice(historicalLogs, func(i, j int) bool {
		numA, _ := strconv.Atoi(re.FindStringSubmatch(historicalLogs[i])[1])
		numB, _ := strconv.Atoi(re.FindStringSubmatch(historicalLogs[j])[1])
		return numA < numB
	})

	for _, fileName := range historicalLogs {
		filePath := filepath.Join(logDir, fileName)
		log.Printf("正在处理历史文件: %s", filePath)
		processSingleFile(filePath, db)
	}
	log.Println("历史日志文件处理完毕。")
}

// processSingleFile 读取并解析单个日志文件
func processSingleFile(filePath string, db *mongo.Database) {
	file, err := os.Open(filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("无法打开日志文件 '%s': %v", filePath, err)
		}
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		parseAndStore(scanner.Text(), db)
	}

	if err := scanner.Err(); err != nil {
		log.Printf("读取日志文件 '%s' 时出错: %v", filePath, err)
	}
}

// watchLogFile 使用 fsnotify 实时监控文件变化
func watchLogFile(filePath string, db *mongo.Database) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalf("创建文件监视器失败: %v", err)
	}
	defer watcher.Close()

	var file *os.File
	var currentPos int64

	openAndSeek := func() {
		var err error
		file, err = os.Open(filePath)
		if err != nil {
			log.Printf("监视期间打开文件 '%s' 失败: %v", filePath, err)
			file = nil
			return
		}
		// 移动到上次读取的位置
		if _, err := file.Seek(currentPos, 0); err != nil {
			log.Printf("移动文件指针失败: %v", err)
		}
	}

	// 初始打开文件并移到末尾，因为 processSingleFile 已经处理了现有内容
	info, err := os.Stat(filePath)
	if err == nil {
		currentPos = info.Size()
	}
	openAndSeek()

	err = watcher.Add(filepath.Dir(filePath))
	if err != nil {
		log.Fatalf("添加文件监视失败: %v", err)
	}

	log.Printf("开始实时监视文件: %s", filePath)

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			if event.Name != filePath {
				continue
			}

			if event.Op&fsnotify.Create == fsnotify.Create {
				log.Println("检测到日志文件被创建 (可能由于日志轮换)，重新打开文件")
				if file != nil {
					file.Close()
				}
				currentPos = 0 // 新文件从头开始
				openAndSeek()
			}

			if event.Op&fsnotify.Write == fsnotify.Write {
				if file == nil {
					openAndSeek()
					if file == nil {
						continue
					}
				}
				scanner := bufio.NewScanner(file)
				for scanner.Scan() {
					parseAndStore(scanner.Text(), db)
				}
				if err := scanner.Err(); err != nil {
					log.Printf("监视期间读取文件出错: %v", err)
				}
				// 更新当前位置
				pos, err := file.Seek(0, os.SEEK_CUR)
				if err == nil {
					currentPos = pos
				}
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Println("监视器错误:", err)
		}
	}
}

func parseAndStore(line string, db *mongo.Database) {
	// 正则表达式保持不变
	committedStateRegex := regexp.MustCompile(`I\[(.*?)\] Committed State\s+module=(.*?)\s+height=(.*?)\s+txs=(.*?)\s+appHash=(.*)`)
	//allocateTokensRegex := regexp.MustCompile(`I\[(.*?)\] Allocate Tokens To Validator\s+module=(.*?)\s+validator=(.*?)\s+reward=(.*)`)
	//executedBlockRegex := regexp.MustCompile(`I\[(.*?)\] Executed Block\s+module=(.*?)\s+height=(.*?)\s+validTxs=(.*?)\s+invalidTxs=(.*?)\s+hash=(.*)`)

	// 解析和存储逻辑保持不变
	if matches := committedStateRegex.FindStringSubmatch(line); len(matches) > 0 {
		timestamp, _ := time.Parse("2006-01-02|15:04:05.000", matches[1])
		height, _ := strconv.ParseInt(matches[3], 10, 64)
		txs, _ := strconv.Atoi(matches[4])

		entry := CommittedState{
			Timestamp: timestamp,
			Module:    matches[2],
			Height:    height,
			Txs:       txs,
			AppHash:   strings.TrimSpace(matches[5]),
		}
		collection := db.Collection("committed_state")
		_, err := collection.InsertOne(context.Background(), entry)
		if err != nil {
			log.Printf("写入 committed_state 到 MongoDB 时出错: %v", err)
		}
	}
}
