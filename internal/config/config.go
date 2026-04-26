package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	DefaultOwner = "029527"
	DefaultRepo  = "gapull"
)

type Config struct {
	Token string `json:"token"`
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "docker-pull-proxy", "config.json"), nil
}

// Load 加载配置：env > 文件 > 默认值
func Load() (*Config, error) {
	cfg := &Config{
		Owner: DefaultOwner,
		Repo:  DefaultRepo,
	}

	// 从文件读取（忽略文件不存在的错误）
	path, err := configPath()
	if err == nil {
		data, err := os.ReadFile(path)
		if err == nil {
			_ = json.Unmarshal(data, cfg)
		}
	}

	// 环境变量优先级最高
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		cfg.Token = t
	}
	if o := os.Getenv("DOCKER_PROXY_OWNER"); o != "" {
		cfg.Owner = o
	}
	if r := os.Getenv("DOCKER_PROXY_REPO"); r != "" {
		cfg.Repo = r
	}

	return cfg, nil
}

// Save 将配置持久化到文件
func Save(cfg *Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// Validate 检查 Token 是否已设置
func (c *Config) Validate() error {
	if c.Token == "" {
		return errors.New("GitHub Token 未设置，请通过 `docker-pull-proxy config set --token <PAT>` 或环境变量 GITHUB_TOKEN 设置")
	}
	return nil
}

func (c *Config) String() string {
	masked := "（未设置）"
	if c.Token != "" {
		if len(c.Token) > 8 {
			masked = c.Token[:4] + "****" + c.Token[len(c.Token)-4:]
		} else {
			masked = "****"
		}
	}
	return fmt.Sprintf("Owner : %s\nRepo  : %s\nToken : %s", c.Owner, c.Repo, masked)
}
