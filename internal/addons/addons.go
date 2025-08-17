package addons

import (
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/RadiantRainbow/wow-addon-cli/internal/util"
	"github.com/go-git/go-git/v6"
	"github.com/rs/zerolog/log"
	"github.com/segmentio/ksuid"
)

const SEP = string(filepath.Separator)
const MARKER = ".wow_addon_cli"

type UnpackCandidate struct {
	TOCFilePath    string
	TOCFilePathDir string
	Depth          int
	Title          string
}

type AddonEntry struct {
	// config strings
	Git  string
	Zip  string
	Url  string
	Name string

	// hydrated later
	UniqueName string
}

func (entry *AddonEntry) Hydrate() error {
	entry.UniqueName = ksuid.New().String()

	if entry.Url != "" {
		u, err := url.Parse(entry.Url)
		if err != nil {
			return err
		}
		ext := filepath.Ext(u.Path)
		switch ext {
		case ".git":
			entry.Git = entry.Url
		case ".zip":
			entry.Zip = entry.Url
		}
	}

	return nil
}

func (entry AddonEntry) CloneSubdirName() string {
	// if entry name is specified, force it to be that!
	if entry.Name != "" {
		return entry.Name
	}

	ext := filepath.Ext(entry.Git)
	if ext == ".git" {
		return filepath.Base(strings.TrimSuffix(entry.Git, ext))
	}

	return filepath.Base(entry.Git)
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

// A unique download path subdir to put artifacts in
func (c Conf) DownloadUniqueDir(entry AddonEntry) (string, error) {
	// TODO check paths? absolute?
	return filepath.Join(c.DownloadPath, entry.UniqueName), nil
}

func (c Conf) DestZip(entry AddonEntry) (string, error) {
	return filepath.Join(c.DownloadPath, entry.UniqueName) + ".zip", nil
}

func FetchEntry(conf Conf, entry AddonEntry) ([]string, error) {
	cleanupPaths := []string{}
	downloadUniqueDir, err := conf.DownloadUniqueDir(entry)
	if err != nil {
		return cleanupPaths, err
	}
	err = os.MkdirAll(downloadUniqueDir, 0755)
	if err != nil {
		return cleanupPaths, err
	}

	cleanupPaths = append(cleanupPaths, downloadUniqueDir)

	if entry.Git != "" {
		clonePath := filepath.Join(downloadUniqueDir, entry.CloneSubdirName())
		log.Debug().Msgf("Entry cloning git: %s to %s", entry.Git, clonePath)

		progressBuf := new(strings.Builder)
		_, err = git.PlainClone(clonePath, &git.CloneOptions{
			URL:      entry.Git,
			Depth:    1,
			Tags:     git.NoTags,
			Progress: progressBuf,
		})

		if err != nil {
			log.Debug().Msgf("Progress buffer output: %s", progressBuf.String())
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

		log.Debug().Msgf("Writing %s to %s", entry.Zip, writePath)
		writtenBytes, err := io.Copy(fp, resp.Body)
		if err != nil {
			return cleanupPaths, err
		}
		log.Debug().Msgf("Wrote %d bytes", writtenBytes)
		cleanupPaths = append(cleanupPaths, writePath)

		// extract do the download unique dir
		destDir := downloadUniqueDir

		err = util.Unzip(writePath, destDir)
		if err != nil {
			return cleanupPaths, err
		}

		log.Debug().Msg("Extraction complete.")
		return cleanupPaths, nil
	}

	return cleanupPaths, fmt.Errorf("nothing to fetch")
}

func UnpackEntry(conf Conf, entry AddonEntry) error {
	log.Debug().Msgf("Unpacking %+v", entry)
	downloadUniqueDir, err := conf.DownloadUniqueDir(entry)
	if err != nil {
		return err
	}

	tocFiles := []*TOCFile{}

	// find the .toc files that mark each addon directory root
	err = filepath.WalkDir(downloadUniqueDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Debug().Msgf("walking error: %v", err)
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

		// enforce rule that <addon_name>/<addon_name>.toc - <addon_name> must match
		//tocFileBasename := filepath.Base(path)
		//tocFileBasename = strings.TrimSuffix(tocFileBasename, filepath.Ext(tocFileBasename))
		//tocDirBasename := filepath.Base(filepath.Dir(path))
		//if tocFileBasename != tocDirBasename {
		//	log.Debug().Msgf("toc file %s does not match the name of the dir: %v", tocFileBasename, tocDirBasename)
		//	return nil
		//}

		// Open the file for reading.
		toc, err := BuildTOCFromFile(path)
		if err != nil {
			log.Warn().Err(err).Msg("error building TOC file, keep walking")
			return nil
		}
		// TODO awkward use of nil and error. have to define what's skippable error
		if toc == nil {
			log.Warn().Msgf("skipped building toc file %+v", path)
			return nil
		}

		tocFiles = append(tocFiles, toc)

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
	// .downloads/12345/Bagnon/Bagnon-3.3.5-main/Bagnon/Bagnon.toc
	// .downloads/12345/Bagnon/Bagnon-3.3.5-main/Bagnon/libs/LibStub/LibStub.toc
	// .downloads/12345/Bagnon/Bagnon-3.3.5-main/Bagnon_Config/Bagnon_Config.toc
	// .downloads/12345/Bagnon/Bagnon-3.3.5-main/Bagnon_Forever/Bagnon_Forever.toc
	// .downloads/12345/Bagnon/Bagnon-3.3.5-main/Bagnon_GuildBank/Bagnon_GuildBank.toc
	// .downloads/12345/Bagnon/Bagnon-3.3.5-main/Bagnon_Tooltips/Bagnon_Tooltips.toc
	// .downloads/12345/Bagnon/Bagnon-3.3.5-main/Bagnon_VoidStorage/Bagnon_VoidStorage.toc
	//
	//  or there could be two tocs in same dir
	// .downloads/6789/Leatrix_Plus/Leatrix_Plus.toc
	// .downloads/6789/Leatrix_Plus/Leatrix_Plus_Wrath.toc

	minDepthTocs := []TOCFile{}

	minDepth := -1
	for _, toc := range tocFiles {
		if minDepth == -1 || toc.Depth < minDepth {
			minDepth = toc.Depth
			continue
		}
	}

	if minDepth == -1 {
		return fmt.Errorf("Could not deterine mindepth")
	}

	for _, toc := range tocFiles {
		if toc.Depth == minDepth {
			minDepthTocs = append(minDepthTocs, *toc)
		}
	}

	log.Debug().Msgf("Min depth tocs %+v", minDepthTocs)

	groups, err := GroupTOCFiles(minDepthTocs)
	if err != nil {
		return err
	}

	log.Debug().Msgf("To unpack toc groups %+v", groups)

	// TODO only keep one of the to unpack entries
	// the the dest addon name dir should be based on the entry's Name if it exists
	// or the parent dir basename if it does not

	for _, grp := range groups {
		addonName, err := grp.AddonName()
		if err != nil {
			log.Warn().Err(err).Msg("Error getting addon name from group")
			continue
		}

		if addonName == "" {
			log.Warn().Msg("Empty addon name")
			continue
		}

		destAddonDir := filepath.Join(conf.AddonsPath, addonName)
		tocSrcDir, err := grp.Dir()
		if err != nil {
			log.Warn().Err(err).Msg("Could not get TOC dir")
			continue
		}

		log.Debug().Msgf("Removing dest dir %v", destAddonDir)
		err = os.RemoveAll(destAddonDir)
		if err != nil {
			return err
		}

		log.Debug().Msgf("Making directories %v", destAddonDir)
		err = os.MkdirAll(destAddonDir, 0755)
		if err != nil {
			return err
		}
		log.Debug().Msgf("Copying %v to %v", tocSrcDir, destAddonDir)
		err = util.CopyDir(destAddonDir, tocSrcDir)
		if err != nil {
			return err
		}

		// create a marker file
		markerDest := filepath.Join(destAddonDir, MARKER)
		log.Debug().Msgf("Creating marker file %s", markerDest)
		_, err = os.Create(markerDest)
		if err != nil {
			return err
		}
	}

	return nil
}

func CleanDownload(conf Conf, cleanupPaths []string) error {
	for _, d := range cleanupPaths {
		log.Debug().Msgf("Removing dir for clean up %v", d)
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

		markerFile := filepath.Join(dir, MARKER)
		log.Debug().Msgf("Checking for marker at %v", markerFile)
		exists, _ := util.FileExists(markerFile)
		if !exists {
			continue
		}

		log.Debug().Msgf("Found marker dir for cleaning %v", dir)

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

		log.Debug().Msgf("Removing marked dir %v", dir)
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
		log.Info().Msgf("Processing entry: %+v", entry)

		// normalize name from Git and other keys
		err := entry.Hydrate()
		if err != nil {
			log.Warn().Err(err).Msgf("error hydrating name, skipping: %+v", entry)
			continue
		}

		if entry.UniqueName == "" {
			log.Warn().Msgf("entry name is empty, skipping: %+v", entry)
			continue
		}

		cleanUpPaths, err := FetchEntry(conf, entry)
		defer func() {
			err := CleanDownload(conf, cleanUpPaths)
			if err != nil {
				log.Error().Err(err).Msgf("error cleaning up download for entry %+v", entry)
			}
		}()
		if err != nil {
			log.Warn().Err(err).Msgf("error fetching entry: %+v, error: %v", entry, err)
			continue
		}

		err = UnpackEntry(conf, entry)
		if err != nil {
			log.Warn().Msgf("error unpacking entry: %+v, error: %v", entry, err)
			continue
		}

		log.Info().Msgf("Done processing entry: %+v", entry)
	}
	return nil
}
