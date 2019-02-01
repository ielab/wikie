package wikie

import (
	"gopkg.in/yaml.v2"
	"os"
)

type OAuth2Config struct {
	ClientID     string `yaml:"clientid"`
	ClientSecret string `yaml:"clientsecret"`
	AuthURL      string `yaml:"authurl"`
	TokenURL     string `yaml:"tokenurl"`
	Redirect     string `yaml:"redirect"`
	State        string `yaml:"state"`
}

type ElasticsearchConfig struct {
	Hosts []string `yaml:"hosts"`
}

type Config struct {
	Port                string   `yaml:"port"`
	RocketChat          string   `yaml:"rocket.chat"`
	Admins              []string `yaml:"admins"`
	CookieSecret        string   `yaml:"cookieSecret"`
	*OAuth2Config       `yaml:"oauth2"`
	ElasticsearchConfig `yaml:"elasticsearch"`
}

func ReadConfig(file string) (config Config, err error) {
	f, err := os.OpenFile(file, os.O_RDONLY, 0644)
	if err != nil {
		return
	}

	err = yaml.NewDecoder(f).Decode(&config)
	return
}
