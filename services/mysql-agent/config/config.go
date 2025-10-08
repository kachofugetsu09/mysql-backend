package config

import (
	"fmt"
	"log"
	"time"

	"github.com/spf13/viper"
)

// Config 全局配置结构体
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Log      LogConfig      `mapstructure:"log"`
	DeepSeek DeepSeekConfig `mapstructure:"deepseek"`
	MySQL    MySQLConfig    `mapstructure:"mysql"`
}

func (c Config) GetServerAddr() string {
	return fmt.Sprintf("%s:%s", c.Server.Host, c.Server.Port)
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Port string `mapstructure:"port"`
	Host string `mapstructure:"host"`
	Mode string `mapstructure:"mode"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	Output string `mapstructure:"output"`
}

// DeepSeekConfig DeepSeek API配置
type DeepSeekConfig struct {
	APIKey          string        `mapstructure:"api_key"`
	BaseURL         string        `mapstructure:"base_url"`
	Model           string        `mapstructure:"model"`
	Timeout         time.Duration `mapstructure:"timeout"`
	AnalysisTimeout time.Duration `mapstructure:"analysis_timeout"`
}

// MySQLConfig MySQL连接配置
type MySQLConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	Database string `mapstructure:"database"`
}

// 全局配置实例
var AppConfig *Config

// InitConfig 初始化配置
func InitConfig() {
	viper.SetConfigName("config")
	viper.SetConfigType("toml")
	viper.AddConfigPath("./config")
	viper.AddConfigPath(".")

	// 设置默认值
	setDefaults()

	// 读取配置文件
	if err := viper.ReadInConfig(); err != nil {
		log.Printf("Error reading config file: %v", err)
		log.Println("Using default configuration")
	} else {
		log.Printf("Using config file: %s", viper.ConfigFileUsed())
	}

	// 解析配置到结构体
	AppConfig = &Config{}
	if err := viper.Unmarshal(AppConfig); err != nil {
		log.Fatalf("Unable to decode into struct: %v", err)
	}

	log.Printf("Configuration loaded successfully")
}

// setDefaults 设置默认配置值
func setDefaults() {
	// 服务器默认配置
	viper.SetDefault("server.port", "8081")
	viper.SetDefault("server.host", "localhost")
	viper.SetDefault("server.mode", "debug")

	// 日志默认配置
	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.format", "json")
	viper.SetDefault("log.output", "stdout")

	// DeepSeek API默认配置
	viper.SetDefault("deepseek.api_key", "")
	viper.SetDefault("deepseek.base_url", "https://api.deepseek.com")
	viper.SetDefault("deepseek.model", "deepseek-chat")
	viper.SetDefault("deepseek.timeout", "120s")
	viper.SetDefault("deepseek.analysis_timeout", "120s")

	// MySQL默认配置
	viper.SetDefault("mysql.host", "localhost")
	viper.SetDefault("mysql.port", 3306)
	viper.SetDefault("mysql.username", "root")
	viper.SetDefault("mysql.password", "")
	viper.SetDefault("mysql.database", "")
}
