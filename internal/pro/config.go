package pro

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/photoprism/photoprism/internal/pro/places"
	"github.com/photoprism/photoprism/pkg/fs"
	"github.com/photoprism/photoprism/pkg/txt"
	"gopkg.in/yaml.v2"
)

// Config represents photoprism.pro api credentials for maps & geodata.
type Config struct {
	Key     string `json:"key" yaml:"key"`
	Secret  string `json:"secret" yaml:"secret"`
	Session string `json:"session" yaml:"session"`
	Status  string `json:"status" yaml:"status"`
	Version string `json:"version" yaml:"version"`
}

// NewConfig creates a new photoprism.pro api credentials instance.
func NewConfig(version string) *Config {
	return &Config{
		Key:     "",
		Secret:  "",
		Session: "",
		Status:  "",
		Version: version,
	}
}

// MapKey returns the maps api key.
func (c *Config) MapKey() string {
	if sess, err := c.DecodeSession(); err != nil {
		return ""
	} else {
		return sess.MapKey
	}
}

// Propagate updates photoprism.pro api credentials in other packages.
func (c *Config) Propagate() {
	places.Key = c.Key
	places.Secret = c.Secret
}

// Sanitize verifies and sanitizes photoprism.pro api credentials.
func (c *Config) Sanitize() {
	c.Key = strings.ToLower(c.Key)

	if c.Secret != "" {
		if c.Key != fmt.Sprintf("%x", sha1.Sum([]byte(c.Secret))) {
			c.Key = ""
			c.Secret = ""
			c.Session = ""
			c.Status = ""
		}
	}
}

// DecodeSession decodes photoprism.pro api session data.
func (c *Config) DecodeSession() (Session, error) {
	c.Sanitize()

	result := Session{}

	if c.Session == "" {
		return result, fmt.Errorf("empty session")
	}

	s, err := hex.DecodeString(c.Session)

	if err != nil {
		return result, err
	}

	hash := sha256.New()
	hash.Write([]byte(c.Secret))

	var b []byte

	block, err := aes.NewCipher(hash.Sum(b))

	if err != nil {
		return result, err
	}

	iv := s[:aes.BlockSize]

	plaintext := make([]byte, len(s))

	stream := cipher.NewCTR(block, iv)
	stream.XORKeyStream(plaintext, s[aes.BlockSize:])

	plaintext = bytes.Trim(plaintext, "\x00")

	if err := json.Unmarshal(plaintext, &result); err != nil {
		return result, err
	}

	return result, nil
}

// Refresh updates photoprism.pro api credentials.
func (c *Config) Refresh() (err error) {
	mutex.Lock()
	defer mutex.Unlock()

	c.Sanitize()
	client := &http.Client{Timeout: 60 * time.Second}
	url := ApiURL
	method := http.MethodPost
	var req *http.Request

	if c.Key != "" {
		url = fmt.Sprintf(ApiURL+"/%s", c.Key)
		method = http.MethodPut
		log.Debugf("pro: updating api key for maps & places")
	} else {
		log.Debugf("pro: requesting api key for maps & places")
	}

	if j, err := json.Marshal(NewRequest(c.Version)); err != nil {
		return err
	} else if req, err = http.NewRequest(method, url, bytes.NewReader(j)); err != nil {
		return err
	}

	req.Header.Add("Content-Type", "application/json")

	var r *http.Response

	for i := 0; i < 3; i++ {
		r, err = client.Do(req)

		if err == nil {
			break
		}
	}

	if err != nil {
		log.Errorf("pro: %s", err.Error())
		return err
	} else if r.StatusCode >= 400 {
		err = fmt.Errorf("api key request for maps & places failed with code %d", r.StatusCode)
		return err
	}

	err = json.NewDecoder(r.Body).Decode(c)

	if err != nil {
		log.Errorf("pro: %s", err.Error())
		return err
	}

	return nil
}

// Load photoprism.pro api credentials from a YAML file.
func (c *Config) Load(fileName string) error {
	if !fs.FileExists(fileName) {
		return fmt.Errorf("api key file not found: %s", txt.Quote(fileName))
	}

	mutex.Lock()
	defer mutex.Unlock()

	yamlConfig, err := ioutil.ReadFile(fileName)

	if err != nil {
		return err
	}

	if err := yaml.Unmarshal(yamlConfig, c); err != nil {
		return err
	}

	c.Sanitize()
	c.Propagate()

	return nil
}

// Save photoprism.pro api credentials to a YAML file.
func (c *Config) Save(fileName string) error {
	mutex.Lock()
	defer mutex.Unlock()

	c.Sanitize()

	data, err := yaml.Marshal(c)

	if err != nil {
		return err
	}

	c.Propagate()

	if err := ioutil.WriteFile(fileName, data, os.ModePerm); err != nil {
		return err
	}

	c.Propagate()

	return nil
}
