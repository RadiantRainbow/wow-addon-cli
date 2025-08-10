package addons

import (
	"bufio"
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

	"github.com/RadiantRainbow/wow-addon-cli/internal/util"
	"github.com/go-git/go-git/v6"
	"github.com/segmentio/ksuid"
)

const SEP = string(filepath.Separator)

type UnpackCandidate struct {
	TOCFilePath string
	SrcPath     string
	DstPath     string
	Title       string
}

type AddonEntry struct {
	Git        string
	Zip        string
	UniqueName string

	// these will be unpacked later
	UnpackMap  map[string]string
	UnpackList []string
}

func (entry *AddonEntry) Hydrate() error {
	entry.UniqueName = ksuid.New().String()
	return nil
}

type Conf struct {
	DownloadPath      string
	BackupPath        string
	AddonsPath        string
	PrecleanBliz      bool
	SkipCleanPrefixes []string
	Addons            []AddonEntry `toml:"addons"`
}

var DefaultSkipCleanPrefixes = []string{
	"Blizzard_",
}

// Where to clone a git repo. Also the final directory of the decompressed archive
func (c Conf) DestDir(entry AddonEntry) (string, error) {
	// TODO check paths? absolute?
	return path.Join(c.DownloadPath, entry.UniqueName), nil
}

func (c Conf) DestZip(entry AddonEntry) (string, error) {
	return path.Join(c.DownloadPath, entry.UniqueName) + ".zip", nil
}

func sanitizeTitle(title string) string {
	// Remove color strings
	// ex.
	// |cff33ffccpf|cffffffffUI
	colorRegex := regexp.MustCompile(`\|cff[a-zA-Z0-9]{3,6}`)
	return colorRegex.ReplaceAllString(title, "")
}

func FetchEntry(conf Conf, entry AddonEntry) ([]string, error) {
	cleanupPaths := []string{}
	destDir, err := conf.DestDir(entry)
	if err != nil {
		return cleanupPaths, err
	}

	cleanupPaths = append(cleanupPaths, destDir)

	if entry.Git != "" {
		clonePath := destDir
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

		err = util.Unzip(writePath, destDir)
		if err != nil {
			return cleanupPaths, err
		}

		log.Println("Extraction complete.")
		return cleanupPaths, nil
	}

	return cleanupPaths, fmt.Errorf("nothing to fetch")
}

func UnpackEntry(conf Conf, entry AddonEntry) error {
	log.Printf("Unpacking %+v", entry)
	destDir, err := conf.DestDir(entry)
	if err != nil {
		return err
	}

	tocFiles := []UnpackCandidate{}

	regexTitle := regexp.MustCompile(`^\s*##\s*Title:\s*(.*)$`)
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

		title := ""
		// TODO capture title and make it the dest dir for Addons
		for scanner.Scan() {
			line := scanner.Text()
			matches := regexTitle.FindStringSubmatch(line)
			if matches != nil {
				containsTitle = true
				title = sanitizeTitle(matches[1])
			}

			if containsInterface == false && regexInterface.MatchString(line) {
				containsInterface = true
			}

		}

		if title == "" {
			log.Printf("Could not parse title for %v", path)
			return nil
		}

		title = strings.ReplaceAll(title, " ", "_")

		if containsTitle && containsInterface {
			log.Printf("Found valid TOC file %v", path)

			candidate := UnpackCandidate{
				TOCFilePath: path,
				Title:       title,
			}
			tocFiles = append(tocFiles, candidate)
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
	unpackCandidateDirMap := map[string]UnpackCandidate{}

	toUnpack := []UnpackCandidate{}

	for _, toc := range tocFiles {
		tocDir := filepath.Dir(toc.TOCFilePath)
		unpackCandidateDirMap[tocDir] = toc
	}

	unpackCandidateDepths := map[string]int{}
	for d := range unpackCandidateDirMap {
		components := strings.Split(filepath.Clean(d), SEP)
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
			toUnpack = append(toUnpack, unpackCandidateDirMap[d])
		}
	}

	log.Printf("To unpack dirs %+v", toUnpack)
	for _, toc := range toUnpack {
		tocDir := filepath.Dir(toc.TOCFilePath)
		destAddonDir := filepath.Join(conf.AddonsPath, toc.Title)

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
		log.Printf("Copying %v to %v", tocDir, destAddonDir)
		err = util.CopyDir(destAddonDir, tocDir)
		if err != nil {
			return err
		}
	}

	return nil
}

func CleanDownload(conf Conf, cleanupPaths []string) error {
	for _, d := range cleanupPaths {
		log.Printf("Removing dir for clean up %v", d)
		err := os.RemoveAll(d)
		if err != nil {
			return err
		}
	}

	return nil
}

func RemoveNonBlizDirs(conf Conf) error {
	matches, err := filepath.Glob(conf.AddonsPath + SEP + "*")
	if err != nil {
		return err
	}
	for _, dir := range matches {
		isDir, err := util.IsDirectory(dir)
		if err != nil {
			return err
		}

		// only clean up dirs
		if !isDir {
			continue
		}

		base := filepath.Base(dir)
		shouldSkip := false
		for _, prefix := range DefaultSkipCleanPrefixes {
			if strings.HasPrefix(base, prefix) {
				shouldSkip = true
			}
		}
		for _, prefix := range conf.SkipCleanPrefixes {
			if strings.HasPrefix(base, prefix) {
				shouldSkip = true
			}
		}
		if strings.HasPrefix(base, ".") {
			shouldSkip = true
		}

		if shouldSkip {
			continue
		}

		log.Printf("Removing non bliz dir %v", dir)
		err = os.RemoveAll(dir)
		if err != nil {
			return err
		}
	}

	return nil
}

func Execute(conf Conf) error {
	err := RemoveNonBlizDirs(conf)
	if err != nil {
		return fmt.Errorf("error cleaning bliz dirs %+v", err)
	}

	for _, entry := range conf.Addons {
		log.Printf("Processing entry: %+v", entry)

		// normalize name from Git and other keys
		err := entry.Hydrate()
		if err != nil {
			return fmt.Errorf("error hydrating name %+v", entry)
		}

		if entry.UniqueName == "" {
			return fmt.Errorf("entry name is empty %+v", entry)
		}

		cleanUpPaths, err := FetchEntry(conf, entry)
		defer CleanDownload(conf, cleanUpPaths)
		if err != nil {
			log.Printf("error fetching entry: %+v, error: %v", entry, err)
			continue
		}

		err = UnpackEntry(conf, entry)
		if err != nil {
			log.Printf("WARN: error unpacking entry: %+v, error: %v", entry, err)
			continue
		}

	}
	return nil
}
