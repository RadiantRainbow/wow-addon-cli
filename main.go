package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/go-git/go-git/v6"
)

type AddonEntry struct {
	Name   string
	Git    string
	Zip    string
	Unpack any

	// these will be unpacked later
	UnpackMap  map[string]string
	UnpackList []string
}

func (entry *AddonEntry) HydrateName() error {
	name := entry.Name
	if entry.Git != "" {
		if name == "" {
			name = path.Base(entry.Git)
			name = strings.TrimSuffix(name, filepath.Ext(name))
		}
		entry.Name = name
	}
	if entry.Zip != "" {
		if name == "" {
			name = path.Base(entry.Zip)
			name = strings.TrimSuffix(name, filepath.Ext(name))
		}
		entry.Name = name
	}
	return nil
}

type Conf struct {
	DownloadPath string
	BackupPath   string
	AddonsPath   string
	Addons       []AddonEntry `toml:"addons"`
}

// Where to clone a git repo. Also the final directory of the decompressed archive
func (c Conf) DestDir(entry AddonEntry) (string, error) {
	// TODO check paths? absolute?
	return path.Join(c.DownloadPath, entry.Name), nil
}

func (c Conf) DestZip(entry AddonEntry) (string, error) {
	return path.Join(c.DownloadPath, entry.Name) + ".zip", nil
}

func fetchEntry(conf Conf, entry AddonEntry) ([]string, error) {
	cleanupPaths := []string{}
	destDir, err := conf.DestDir(entry)
	if err != nil {
		return cleanupPaths, err
	}

	cleanupPaths = append(cleanupPaths, destDir)

	if entry.Git != "" {
		clonePath := destDir
		if err != nil {
			return cleanupPaths, err
		}

		log.Printf("Entry cloning git: %s to %s", entry.Git, clonePath)

		_, err = git.PlainClone(clonePath, &git.CloneOptions{
			URL:      entry.Git,
			Depth:    1,
			Tags:     git.NoTags,
			Progress: os.Stdout,
		})

		if err != nil {
			return cleanupPaths, err
		}

		return cleanupPaths, err
	}

	if entry.Zip != "" {
		client := http.Client{
			Timeout: time.Second * 20,
		}

		resp, err := client.Get(entry.Zip)
		if err != nil {
			return cleanupPaths, err
		}
		defer resp.Body.Close()

		writePath, err := conf.DestZip(entry)
		if err != nil {
			return cleanupPaths, err
		}
		fp, err := os.Create(writePath)
		if err != nil {
			return cleanupPaths, err
		}
		defer fp.Close()

		log.Printf("Writing %s to %s", entry.Zip, writePath)
		writtenBytes, err := io.Copy(fp, resp.Body)
		if err != nil {
			return cleanupPaths, err
		}
		log.Printf("Wrote %d bytes", writtenBytes)
		cleanupPaths = append(cleanupPaths, writePath)

		destDir, err := conf.DestDir(entry)
		if err != nil {
			return cleanupPaths, err
		}
		err = os.MkdirAll(destDir, 0755)
		if err != nil {
			return cleanupPaths, err
		}

		err = unzip(writePath, destDir)
		if err != nil {
			return cleanupPaths, err
		}
		log.Println("Extraction complete.")
	}

	// TODO error
	return cleanupPaths, err
}

func unpackEntry(conf Conf, entry AddonEntry) error {
	log.Printf("Unpacking %+v", entry)
	destDir, err := conf.DestDir(entry)
	if err != nil {
		return err
	}

	tocFiles := []string{}

	regexTitle := regexp.MustCompile(`^\s*##\s*Title:.*$`)
	regexInterface := regexp.MustCompile(`^\s*##\s*Interface:.*$`)

	// find the .toc files that mark each addon directory root
	err = filepath.WalkDir(destDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Printf("walking error: %v", err)
			return err
		}

		if !strings.HasSuffix(d.Name(), ".toc") {
			// only keep TOC files
			return nil
		}

		// Skip directories, we only want to search in files.
		if d.IsDir() {
			return nil
		}

		// Open the file for reading.
		file, err := os.Open(path)
		if err != nil {
			log.Printf("Could not open file %s: %v", path, err)
			return nil // Continue walking
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		containsTitle := false
		containsInterface := false
		for scanner.Scan() {
			line := scanner.Text()
			if containsTitle == false && regexTitle.Match([]byte(line)) {
				containsTitle = true
			}
			if containsInterface == false && regexInterface.Match([]byte(line)) {
				containsInterface = true
			}
		}

		if containsTitle && containsInterface {
			log.Printf("Found valid TOC file %v", path)
			tocFiles = append(tocFiles, path)
		}

		// Check for errors during scanning.
		if err := scanner.Err(); err != nil {
			log.Printf("Error scanning file %s: %v", path, err)
		}

		return nil
	})

	if err != nil {
		return err
	}

	if len(tocFiles) == 0 {
		return fmt.Errorf("No toc files detected, nothing to unpack")
	}

	// find the "shallowest" .toc file which becomes the "root"
	// all dirs in this root should be unpacked to target dir
	// ex
	// .downloads/Bagnon/Bagnon-3.3.5-main/Bagnon/Bagnon.toc
	// .downloads/Bagnon/Bagnon-3.3.5-main/Bagnon/libs/LibStub/LibStub.toc
	// .downloads/Bagnon/Bagnon-3.3.5-main/Bagnon_Config/Bagnon_Config.toc
	// .downloads/Bagnon/Bagnon-3.3.5-main/Bagnon_Forever/Bagnon_Forever.toc
	// .downloads/Bagnon/Bagnon-3.3.5-main/Bagnon_GuildBank/Bagnon_GuildBank.toc
	// .downloads/Bagnon/Bagnon-3.3.5-main/Bagnon_Tooltips/Bagnon_Tooltips.toc
	// .downloads/Bagnon/Bagnon-3.3.5-main/Bagnon_VoidStorage/Bagnon_VoidStorage.toc
	unpackCandidateDirs := []string{}
	toUnpack := []string{}
	for _, tocFilePath := range tocFiles {
		tocDir := filepath.Dir(tocFilePath)
		unpackCandidateDirs = append(unpackCandidateDirs, tocDir)
	}

	unpackCandidateDepths := map[string]int{}
	for _, d := range unpackCandidateDirs {
		components := strings.Split(filepath.Clean(d), string(filepath.Separator))
		unpackCandidateDepths[d] = len(components)
	}

	log.Printf("Unpack candidate depths %+v", unpackCandidateDepths)

	minDepth := -1
	for d, depth := range unpackCandidateDepths {
		if minDepth == -1 || depth < minDepth {
			log.Printf("Set min depth to %d for dir %v", depth, d)
			minDepth = depth
			continue
		}
	}

	if minDepth == -1 {
		return fmt.Errorf("Could not deterine mindepth")
	}

	for d, depth := range unpackCandidateDepths {
		if depth == minDepth {
			toUnpack = append(toUnpack, d)
		}
	}

	log.Printf("To unpack dirs %+v", toUnpack)
	for _, d := range toUnpack {
		destAddonDir := filepath.Join(conf.AddonsPath, filepath.Base(d))

		log.Printf("Removing dest dir %v", destAddonDir)
		err := os.RemoveAll(destAddonDir)
		if err != nil {
			return err
		}

		log.Printf("Making directories %v", destAddonDir)
		err = os.MkdirAll(destAddonDir, 0755)
		if err != nil {
			return err
		}
		log.Printf("Copying %v to %v", d, destAddonDir)
		err = CopyDir(destAddonDir, d)
		if err != nil {
			return err
		}
	}

	return nil
}

func cleanDownload(conf Conf, cleanupPaths []string) error {
	for _, d := range cleanupPaths {
		log.Printf("Cleaning up %v", d)
		err := os.RemoveAll(d)
		if err != nil {
			return err
		}
	}

	return nil
}

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

	var conf Conf
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

	for _, entry := range conf.Addons {
		log.Printf("Processing entry: %+v", entry)

		// normalize name from Git and other keys
		err := entry.HydrateName()
		if err != nil {
			log.Fatalf("ERROR: entry name is empty %+v", entry)
		}

		if entry.Name == "" {
			log.Fatalf("ERROR: entry name is empty %+v", entry)
			continue
		}

		cleanUpPaths, err := fetchEntry(conf, entry)
		if err != nil {
			log.Printf("error fetching entry: %+v, error: %v", entry, err)
			continue
		}

		defer cleanDownload(conf, cleanUpPaths)

		err = unpackEntry(conf, entry)
		if err != nil {
			log.Printf("WARN: error unpacking entry: %+v, error: %v", entry, err)
			continue
		}

	}
}
