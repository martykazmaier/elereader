package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"elereader/internal/door32"
	"elereader/internal/jam"
	"elereader/internal/ra"
	"elereader/internal/ui"
)

func main() {
	doorPath := flag.String("door", "", "path to DOOR32.SYS (default: DOOR32.SYS in the working directory)")
	cfgPath := flag.String("cfg", "", "path to elereader.cfg (default: beside the exe)")
	basePath := flag.String("base", "", "open this JAM base instead of the config file")
	baseName := flag.String("name", "Messages", "area name used with -base")
	baseType := flag.String("type", "local", "area type used with -base: local, echo, netmail, email")
	local := flag.Bool("local", false, "local console, no drop file")
	demo := flag.Bool("demo", false, "open a built-in sample area on the local console")
	userName := flag.String("user", "Sysop", "name used with -local or -demo")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Elereader — full-screen lightbar message reader for EleBBS.\n\n")
		fmt.Fprintf(os.Stderr, "Build the 32-bit exe with build.bat so the Win32 SOCKET in DOOR32.SYS can be used.\n")
		fmt.Fprintf(os.Stderr, "EleBBS should run elereader.exe directly (not from a batch file),\n")
		fmt.Fprintf(os.Stderr, "with the node directory as the working directory.\n")
		fmt.Fprintf(os.Stderr, "MESSAGES.RA is read from the ELEBBS directory, or RA if ELEBBS is not set.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if runtime.GOOS != "windows" || runtime.GOARCH != "386" {
		fmt.Fprintf(os.Stderr, "elereader must be a 32-bit Windows program (windows/386). This binary is %s/%s.\n", runtime.GOOS, runtime.GOARCH)
		fmt.Fprintln(os.Stderr, "Build it with build.bat.")
		os.Exit(1)
	}

	if *demo {
		*local = true
	}

	drop, err := loadDrop(*local, *doorPath, flag.Args(), *userName)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	areas, fail, err := loadAreas(*cfgPath, *basePath, *baseName, *baseType, *demo, nodeDir(drop))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	port, cleanup, err := door32.Open(drop)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer cleanup()
	defer port.Close()

	ui.Run(port, drop, areas, fail)
}

func loadDrop(local bool, doorFlag string, args []string, user string) (door32.Drop, error) {
	if local {
		return door32.Drop{
			CommType:  door32.CommLocal,
			RealName:  user,
			Alias:     user,
			Emulation: door32.EmuANSI,
			Node:      1,
			BBSID:     "local",
			Started:   time.Now(),
		}, nil
	}
	path := doorFlag
	if path == "" && len(args) == 1 {
		fi, err := os.Stat(args[0])
		if err == nil && fi.IsDir() {
			path = filepath.Join(args[0], "DOOR32.SYS")
		} else {
			path = args[0]
		}
	}
	if path == "" {
		for _, name := range []string{"DOOR32.SYS", "door32.sys"} {
			if _, err := os.Stat(name); err == nil {
				path = name
				break
			}
		}
	}
	if path == "" {
		path = "DOOR32.SYS"
	}
	return door32.Parse(path)
}

func nodeDir(drop door32.Drop) string {
	if wd, err := os.Getwd(); err == nil && wd != "" {
		return wd
	}
	if drop.Path != "" {
		return filepath.Dir(drop.Path)
	}
	return "."
}

func loadAreas(cfgFlag, base, name, kindName string, demo bool, nodeDir string) ([]ui.Area, string, error) {
	opts := readCfg(cfgPath(cfgFlag))
	if demo {
		dir, err := os.MkdirTemp("", "elereader")
		if err != nil {
			return nil, "", err
		}
		path, err := jam.WriteDemo(dir)
		if err != nil {
			return nil, "", err
		}
		area := ui.Area{Name: "General", Path: path, Kind: jam.AreaEcho, Node: nodeDir}
		applyCfg(&area, opts)
		return []ui.Area{area}, "", nil
	}
	if base != "" {
		if name == "" {
			name = "Messages"
		}
		kind, _ := jam.ParseKind(kindName)
		area := ui.Area{Name: name, Path: jamBase(base), Kind: kind, Node: nodeDir}
		applyCfg(&area, opts)
		return []ui.Area{area}, "", nil
	}
	if area, err := ra.Current(nodeDir); err == nil {
		applyCfg(&area, opts)
		return []ui.Area{area}, "", nil
	} else if opts.err != nil {
		return nil, "", opts.err
	} else if len(opts.areas) > 0 {
		area := opts.areas[0]
		if area.Node == "" {
			area.Node = nodeDir
		}
		applyCfg(&area, opts)
		return []ui.Area{area}, "", nil
	} else {
		return nil, err.Error(), nil
	}
}

func applyCfg(area *ui.Area, opts cfgFile) {
	if area.Editor == "" {
		area.Editor = opts.editor
	}
	if area.Fetch == "" {
		area.Fetch = opts.fetch
	}
}

func cfgPath(flagPath string) string {
	if flagPath != "" {
		return flagPath
	}
	exe, err := os.Executable()
	if err != nil {
		return "elereader.cfg"
	}
	return filepath.Join(filepath.Dir(exe), "elereader.cfg")
}

type cfgFile struct {
	areas  []ui.Area
	editor string
	fetch  string
	err    error
}

func readCfg(path string) cfgFile {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfgFile{}
		}
		return cfgFile{err: err}
	}
	defer f.Close()
	dir := filepath.Dir(path)
	var out cfgFile
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		low := strings.ToLower(line)
		switch {
		case strings.HasPrefix(low, "editor="), strings.HasPrefix(low, "editor "):
			out.editor = strings.Trim(strings.TrimSpace(line[7:]), `"'`)
			continue
		case strings.HasPrefix(low, "download="), strings.HasPrefix(low, "download "):
			out.fetch = strings.Trim(strings.TrimSpace(line[9:]), `"'`)
			continue
		case strings.HasPrefix(low, "area "):
			line = strings.TrimSpace(line[5:])
		case strings.HasPrefix(low, "area="):
			line = strings.TrimSpace(line[5:])
		}
		i := strings.IndexByte(line, '=')
		if i <= 0 {
			continue
		}
		areaName := strings.TrimSpace(line[:i])
		areaPath := strings.Trim(strings.TrimSpace(line[i+1:]), `"'`)
		if areaName == "" || areaPath == "" {
			continue
		}
		kind := jam.AreaLocal
		fields := strings.Fields(areaPath)
		if len(fields) >= 2 {
			if k, ok := jam.ParseKind(fields[len(fields)-1]); ok {
				kind = k
				areaPath = strings.Join(fields[:len(fields)-1], " ")
			}
		}
		if !filepath.IsAbs(areaPath) {
			areaPath = filepath.Join(dir, areaPath)
		}
		out.areas = append(out.areas, ui.Area{Name: areaName, Path: areaPath, Kind: kind, Node: dir})
	}
	out.err = sc.Err()
	return out
}

func jamBase(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jhr", ".jdt", ".jdx", ".jlr":
		return strings.TrimSuffix(path, filepath.Ext(path))
	default:
		return path
	}
}
