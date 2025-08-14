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
	flagConfig := flag.String("config", "config.toml", "config file")
	flagDownloadPath := flag.String("dlpath", ".downloads", "download path")
	flagBackupPath := flag.String("backuppath", ".backups", "download path")
	flagAddonsPath := flag.String("addonspath", ".", "path to AddOns")
	flagNoPreclean := flag.Bool("nopreclean", true, "skip cleaning non Blizzard addons before fetching")
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

	basenameAddonsPath := filepath.Base(conf.AddonsPath)
	if !(basenameAddonsPath == "AddOns" || basenameAddonsPath == "Addons") {
		log.Fatalf("Addons path %v does not look like an addons path. Expecting 'AddOns' or 'Addons'", conf.AddonsPath)
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

	preCleanBliz := true
	if *flagNoPreclean {
		preCleanBliz = false
	}
	conf.PrecleanBliz = preCleanBliz

	log.Printf("Running with conf: %+v", conf)
	err = addons.Execute(conf)
	if err != nil {
		log.Fatal(err)
	}
}
