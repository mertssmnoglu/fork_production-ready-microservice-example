package config

import (
	"fmt"

	"github.com/spf13/viper"
)

type AppConfig struct {
	Port              string `mapstructure:"port" yaml:"port"`
	HttpServer        string `mapstructure:"http_server" yaml:"http_server"`
	CouchbaseUrl      string `mapstructure:"couchbase_url" yaml:"couchbase_url"`
	OtelTraceEndpoint string `mapstructure:"otel_trace_endpoint" yaml:"otel_trace_endpoint"`
	CouchbaseUsername string `mapstructure:"couchbase_username" yaml:"couchbase_username"`
	CouchbasePassword string `mapstructure:"couchbase_password" yaml:"couchbase_password"`
}

func Read() *AppConfig {
	viper.SetConfigName("config")      // name of config file (without extension)
	viper.SetConfigType("yaml")        // REQUIRED if the config file does not have the extension in the name
	viper.AddConfigPath("$PWD/config") // call multiple times to add many search paths
	viper.AddConfigPath(".")           // optionally look for config in the working directory
	viper.AddConfigPath("/config")     // optionally look for config in the working directory
	viper.AddConfigPath("./config")    // optionally look for config in the working directory
	err := viper.ReadInConfig()        // Find and read the config file
	if err != nil {                    // Handle errors reading the config file
		panic(fmt.Errorf("fatal error config file: %w", err))
	}

	var appConfig AppConfig
	err = viper.Unmarshal(&appConfig)
	if err != nil {
		panic(fmt.Errorf("fatal error unmarshalling config: %w", err))
	}

	return &appConfig
}
