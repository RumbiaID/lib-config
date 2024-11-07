package lib_config

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/joho/godotenv"
)

func SetupConfig(configFile []byte) {
	slog.Info("Setup Configuration")
	conf, err := readConf(configFile)
	if err != nil {
		slog.Error("failed to load config", "err", err.Error())
		return
	}
	setup(conf)
}

type config struct {
	Key          string `json:"key" yaml:"key"`
	DefaultValue string `json:"default" yaml:"default"`
	IsRequired   bool   `json:"is_required" yaml:"is_required"`
	Description  string `json:"description" yaml:"description"`
}

func readConf(file []byte) ([]config, error) {
	if file == nil {
		slog.Info("no config file provided")
		return make([]config, 0), nil
	}
	var data []config
	err := yaml.Unmarshal(file, &data)
	if err != nil {
		slog.Error("failed to decode config", "error", err.Error())
		return nil, err
	}
	return data, nil
}

func setup(configs []config) {
	failedSetupConf := false
	dataENVFile := map[string][]string{}
	dataENVExampleFile := map[string][]string{}

	if err := godotenv.Load(); err != nil {
		slog.Info("Failed to load .env file, using environment variables")
	}
	for _, conf := range configs {
		val := os.Getenv(conf.Key)
		if val == "" {
			val = conf.DefaultValue
			if conf.IsRequired && val == "" {
				slog.Error(fmt.Sprintf("Environment variable '%s' is required.", conf.Key), "description", conf.Description)
				failedSetupConf = true
			} else if !conf.IsRequired {
				if val == "" {
					slog.Info(fmt.Sprintf("Environment variable '%s' is not set.", conf.Key), "description", conf.Description)
				} else {
					slog.Info(fmt.Sprintf("Environment variable '%s' not provided, set default '%s'.", conf.Key, conf.DefaultValue), "description", conf.Description)
				}
			}
		}
		if val != "" {
			if err := os.Setenv(conf.Key, val); err != nil {
				slog.Error("Failed to set environment variable.", "env_key", conf.Key, "value", val, "err", err)
				failedSetupConf = true
			} else {
				slog.Info("Success to set environment variable.", "env_key", conf.Key, "value", val, "description", conf.Description)
			}
		}

		keyPrefix := strings.Split(conf.Key, "_")[0]

		fieldENV := fmt.Sprintf("%s=\"%s\"", conf.Key, val)
		fieldENVExample := fmt.Sprintf("%s=\"%s\"", conf.Key, conf.DefaultValue)

		if val == "" && conf.IsRequired {
			fieldENV += _addSpace(fieldENV, 0) + "# REQUIRED"
			fieldENVExample += _addSpace(fieldENVExample, 0) + "# REQUIRED"
		} else if conf.DefaultValue != "" {
			fieldENV += fmt.Sprintf("%s# DEFAULT:\"%s\"", _addSpace(fieldENV, 0), conf.DefaultValue)
			fieldENVExample += fmt.Sprintf("%s# DEFAULT:\"%s\"", _addSpace(fieldENVExample, 0), conf.DefaultValue)
		}

		if conf.Description != "" {
			fieldENV += fmt.Sprintf("%s# %s", _addSpace(fieldENV, 1), conf.Description)
			fieldENVExample += fmt.Sprintf("%s# %s", _addSpace(fieldENVExample, 1), conf.Description)
		}

		fieldENV += "\n"
		fieldENVExample += "\n"

		_, ok := dataENVFile[keyPrefix]
		if !ok {
			dataENVFile[keyPrefix] = []string{fieldENV}
		} else {
			dataENVFile[keyPrefix] = append(dataENVFile[keyPrefix], fieldENV)
		}

		_, ok = dataENVExampleFile[keyPrefix]
		if !ok {
			dataENVExampleFile[keyPrefix] = []string{fieldENVExample}
		} else {
			dataENVExampleFile[keyPrefix] = append(dataENVExampleFile[keyPrefix], fieldENVExample)
		}
	}

	if err := _genFileENV(".env", dataENVFile); err != nil {
		os.Exit(1)
	}

	if err := _genFileENV(".env.example", dataENVExampleFile); err != nil {
		os.Exit(1)
	}

	if failedSetupConf {
		os.Exit(1)
	}
}

func _addSpace(field string, level int) string {
	baseSpace := 40
	multiple := 20
	space := baseSpace
	lengthField := len(field)
	for {
		baseSpace += level * multiple
		if (lengthField + 2) < baseSpace {
			return strings.Repeat(" ", baseSpace-lengthField)
		} else if (lengthField + 2) < (space + multiple) {
			return strings.Repeat(" ", (space+multiple)-lengthField)
		}
		space += multiple
	}
}

func _genFileENV(filename string, configs map[string][]string) error {
	var keys []string
	for k := range configs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	user, err := exec.Command("git", "config", "user.name").Output()
	if err != nil {
		hostName, err := os.Hostname()
		if err == nil {
			user = []byte(hostName + "\n")
		} else {
			user = []byte("unknown")
		}
	}
	dataEnv := fmt.Sprintf("# Updated At %s \n# Updated By %s\n", time.Now().Format(time.RFC850), string(user))
	for _, key := range keys {
		for _, cfg := range configs[key] {
			dataEnv += cfg
		}
		dataEnv += "\n"
	}

	fileEnv, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			f, err := _createFile(filename, []byte(dataEnv))
			if err != nil {
				return err
			}
			f.Close()
		} else {
			slog.Error(fmt.Sprintf("Failed to read %s file", filename), "error", err.Error())
			return err
		}
	} else {
		if strings.SplitN(string(fileEnv), "\n\n", 2)[1] != strings.SplitN(dataEnv, "\n\n", 2)[1] {
			if err := os.Remove(filename); err != nil {
				slog.Error(fmt.Sprintf("Failed to remove %s file", filename), "error", err.Error())
				return err
			}
			f, err := _createFile(filename, []byte(dataEnv))
			if err != nil {
				return err
			}
			f.Close()
			slog.Info(fmt.Sprintf("Recreate file %s", filename))
		}
	}
	return nil
}

func _createFile(filename string, data []byte) (*os.File, error) {
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		slog.Error(fmt.Sprintf("Failed to create %s file", filename), "error", err.Error())
		return nil, err
	}
	_, err = f.Write(data)
	if err != nil {
		slog.Error(fmt.Sprintf("Failed to write to %s file", filename), "error", err.Error())
		return nil, err
	}
	return f, nil
}

func Set(key, value string) {
	_ = os.Setenv(key, value)
	slog.Info(fmt.Sprintf("Setting '%s' to '%s'", key, value))
}

func GetInt(key string) int {
	val, ok := os.LookupEnv(key)
	if !ok {
		return 0
	}
	valInt, err := strconv.Atoi(val)
	if err != nil {
		return 0
	}
	return valInt
}

func GetInt64(key string) int64 {
	return int64(GetInt(key))
}

func GetString(key string) string {
	val, ok := os.LookupEnv(key)
	if !ok {
		return ""
	}
	return val
}

func GetBool(key string) bool {
	val, ok := os.LookupEnv(key)
	if !ok {
		return false
	}
	return val == "true"
}

func GetListString(key string) []string {
	val, ok := os.LookupEnv(key)
	if !ok {
		return []string{}
	}
	return strings.Split(val, ",")
}

func GetDuration(key string) time.Duration {
	val, ok := os.LookupEnv(key)
	if !ok {
		return 0
	}
	duration, err := time.ParseDuration(val)
	if err != nil {
		return 0
	}
	return duration
}

func GetSize(key string) int64 {
	val, ok := os.LookupEnv(key)
	if !ok {
		return 0
	}

	fmt.Println(val)

	value := "0"
	format := "b"

	formatMap := []string{"b", "kb", "mb", "gb", "tb", "pb"}

	val = strings.ToLower(strings.TrimSpace(val))
	fmt.Println(val)
	if val[len(val)-1] == 'b' {
		switch val[len(val)-2:] {
		case "kb":
			value = val[:len(val)-2]
			format = "kb"
		case "mb":
			value = val[:len(val)-2]
			format = "mb"
		case "gb":
			value = val[:len(val)-2]
			format = "gb"
		case "tb":
			value = val[:len(val)-2]
			format = "tb"
		default:
			value = val
		}
	}

	fmt.Println(value, format)

	valN, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}

	for i, v := range formatMap {
		if v == format {
			fmt.Println(valN, "x", i+1, "x", 1024)
			return valN * (int64(i) + 1) * 1024
		}
	}

	return 0
}
