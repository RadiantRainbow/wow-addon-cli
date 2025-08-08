package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/RadiantRainbow/wow-addon-cli/internal/addons"
)

func main() {
	flagConfig := flag.String("config", "", "config file")
	flagDownloadPath := flag.String("dlpath", ".downloads", "download path")
	flagBackupPath := flag.String("backuppath", ".backups", "download path")
	flagAddonsPath := flag.String("addonspath", ".", "path to AddOns")
	flag.Parse()

	confData, err := os.ReadFile(*flagConfig)
	if err != nil {
		log.Fatal(err)
	}

	var conf addons.Conf
	_, err = toml.Decode(string(confData), &conf)
	if err != nil {
		log.Fatal(err)
	}

	conf.AddonsPath, err = filepath.Abs(*flagAddonsPath)
	if err != nil {
		log.Fatal(err)
	}

	if filepath.Base(conf.AddonsPath) != "AddOns" {
		log.Fatal("Addons path %v does not look like an addons path. Expecting 'AddOns'", conf.AddonsPath)
	}

	// change directories to AddOns so relative default paths work
	err = os.Chdir(conf.AddonsPath)
	if err != nil {
		log.Fatal(err)
	}

	conf.BackupPath, err = filepath.Abs(*flagBackupPath)
	if err != nil {
		log.Fatal(err)
	}
	conf.DownloadPath, err = filepath.Abs(*flagDownloadPath)
	if err != nil {
		log.Fatal(err)
	}

	err = addons.Execute(conf)
	if err != nil {
		log.Fatal(err)
	}
}
