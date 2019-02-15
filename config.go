package wikie

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"os"
)

type OAuth2Config struct {
	ClientID     string   `yaml:"clientid"`
	ClientSecret string   `yaml:"clientsecret"`
	AuthURL      string   `yaml:"authurl"`
	TokenURL     string   `yaml:"tokenurl"`
	Redirect     string   `yaml:"redirect"`
	State        string   `yaml:"state"`
	Scopes       []string `yaml:"scopes"`
	Enabled      bool     `yaml:"enabled"`
	Provider     string   `yaml:"provider"`
}

type RocketChatConfig struct {
	URL     string `yaml:"url"`
	Enabled bool   `yaml:"enabled"`
}

type ElasticsearchConfig struct {
	Hosts []string `yaml:"hosts"`
}

type Config struct {
	Port                string              `yaml:"port"`
	RocketChatConfig    RocketChatConfig    `yaml:"rocket.chat"`
	Admins              []string            `yaml:"admins"`
	CookieSecret        string              `yaml:"cookieSecret"`
	OAuth2Config        *OAuth2Config       `yaml:"oauth2"`
	ElasticsearchConfig ElasticsearchConfig `yaml:"elasticsearch"`
}

func ReadConfig(file string) (config Config, err error) {
	f, err := os.OpenFile(file, os.O_RDONLY, 0644)
	if err != nil {
		return
	}

	err = yaml.NewDecoder(f).Decode(&config)
	if err != nil {
		return
	}

	if config.OAuth2Config != nil && config.OAuth2Config.Enabled && config.OAuth2Config.Provider != "Google" {
		err = fmt.Errorf("only `Google` provider is supported currently")
		return
	}

	return
}
