package addons

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/RadiantRainbow/wow-addon-cli/internal/util"
	"github.com/rs/zerolog/log"
)

type TOCFile struct {
	Path        string
	Dir         string
	Basename    string
	DirBasename string
	Depth       int
}

// NameNoClientSuffix returns the toc addon name with the extension removed
// and the client suffix trimmed
func (toc TOCFile) AddonNameNoClientSuffix() string {
	name := util.RemoveExt(toc.Basename)
	for _, suf := range CLIENT_SUFFIXES {
		if strings.Contains(name, suf) {
			return strings.TrimSuffix(name, suf)
		}
	}

	return name
}

type TOCFileGroup struct {
	TOCFiles []TOCFile
}

var CLIENT_SUFFIXES = []string{
	"_Wrath",
	"_TBC",
	"-tbc",
	"_Vanilla",
	"_Mainline",
	"_Cata",
	"-WOTLKC",
	"-BCC",
	"-Classic",
}

func (grp TOCFileGroup) AddonName() (string, error) {
	if len(grp.TOCFiles) == 0 {
		return "", fmt.Errorf("0 files in group, no addon name")
	}

	if len(grp.TOCFiles) == 1 {
		return util.RemoveExt(grp.TOCFiles[0].Basename), nil
	}

	// trim version specific suffixes
	name := ""
	for _, toc := range grp.TOCFiles {
		trimmed := toc.AddonNameNoClientSuffix()
		if name == "" {
			name = trimmed
			continue
		}

		if name != trimmed {
			return "", fmt.Errorf("found addon name in group that does not match others expected name: %v, other name: %v", name, trimmed)
		}
	}

	// all trimmed names should be equal

	return name, nil
}

func (grp TOCFileGroup) Dir() (string, error) {
	if len(grp.TOCFiles) == 0 {
		return "", fmt.Errorf("0 files in group, no addon name")
	}

	return grp.TOCFiles[0].Dir, nil
}

// sanitizeTitle removes stuff like color strings
// ex.
// |cff33ffccpf|cffffffffUI
// |cffff8000WOW-HC.com|r
func sanitizeTitle(title string) string {
	colorRegex := regexp.MustCompile(`\|cff[a-zA-Z0-9]{3,6}`)
	colorRegexReset := regexp.MustCompile(`\|r`)
	t := title
	t = colorRegex.ReplaceAllString(t, "")
	t = colorRegexReset.ReplaceAllString(t, "")

	return t
}

// sanitizeTocLine removes invalid or bad characters that mess with regexes
func sanitizeTocLine(l string) string {
	l = strings.ReplaceAll(l, "\ufeff", "")
	return l
}

func BuildTOCFromFile(path string) (*TOCFile, error) {
	file, err := os.Open(path)
	if err != nil {
		log.Debug().Msgf("Could not open file %s: %v", path, err)
		return nil, err
	}
	defer file.Close()

	regexTitle := regexp.MustCompile(`^\s*##\s*Title:\s*(.*)$`)
	regexInterface := regexp.MustCompile(`^\s*##\s*Interface:.*$`)

	scanner := bufio.NewScanner(file)
	containsTitle := false
	containsInterface := false

	title := ""
	for scanner.Scan() {
		if containsTitle && containsInterface {
			break
		}

		line := scanner.Text()
		line = sanitizeTocLine(line)
		if containsTitle == false {
			matches := regexTitle.FindStringSubmatch(line)
			if matches != nil {
				containsTitle = true
				title = sanitizeTitle(matches[1])
			}
		}

		if containsInterface == false {
			containsInterface = regexInterface.MatchString(line)
		}
	}

	// Check for errors during scanning.
	if err := scanner.Err(); err != nil {
		log.Debug().Msgf("Error scanning file %s: %v", path, err)
		return nil, err
	}

	if title == "" {
		log.Debug().Msgf("Could not parse title for %v", path)
		return nil, nil
	}

	if !(containsTitle && containsInterface) {
		log.Warn().Msgf("Invalid TOC file, does not contain interface or title %+v", path)
		return nil, nil
	}

	d := filepath.Dir(path)
	components := strings.Split(filepath.Clean(d), SEP)
	basename := filepath.Base(path)
	dirBasename := filepath.Base(d)

	toc := &TOCFile{
		Path:        path,
		Basename:    basename,
		Dir:         d,
		DirBasename: dirBasename,
		Depth:       len(components),
	}

	log.Debug().Msgf("Done reading valid TOC file %+v", toc)

	return toc, nil
}

// GroupTOCFiles takes toc file structs and collects them into lists based on their common
// matching dirs
func GroupTOCFiles(tocs []TOCFile) ([]TOCFileGroup, error) {
	groups := []TOCFileGroup{}

	grpDirMap := map[string]TOCFileGroup{}

	for _, toc := range tocs {
		grp, ok := grpDirMap[toc.Dir]
		if !ok {
			newGrp := TOCFileGroup{
				TOCFiles: []TOCFile{toc},
			}
			grpDirMap[toc.Dir] = newGrp

			continue
		}

		if ok {
			grp.TOCFiles = append(grp.TOCFiles, toc)
			// need to set back the grp after append
			grpDirMap[toc.Dir] = grp
		}
	}

	for _, g := range grpDirMap {
		groups = append(groups, g)
	}
	return groups, nil
}
