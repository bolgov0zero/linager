package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// maxUnpacked bounds what an archive may unpack to (the signed size of the archive is already bounded).
const maxUnpacked = 1 << 30

// Archive layouts (see .github/workflows/release.yml):
//
//	macOS    linager-desktop_macos_arm64.zip       Linager.app/...
//	Windows  linager-desktop-autoupdate_windows_x64.zip       linager-app.exe
//	Linux    linager-desktop_linux_x64.tar.gz      linager-app
const (
	winExe   = "linager-app.exe"
	linuxExe = "linager-app"
)

// launch starts the installed program: on macOS the bundle is opened (a second copy next to the old one), elsewhere the
// file. The new copy is told to wait for this one to exit ("-after"): the program keeps a lock of the single instance.
func launch(path string) error {
	pid := strconv.Itoa(os.Getpid())
	if runtime.GOOS == "darwin" {
		return exec.Command("open", "-n", path, "--args", "-after", pid).Run()
	}
	cmd := exec.Command(path, "-after", pid)
	cmd.Dir = filepath.Dir(path)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// apply puts the program from the archive in place of the running one at exe and starts it.
func apply(goos, archive, exe string, launch func(string) error) error {
	switch goos {
	case "darwin":
		return applyDarwin(archive, exe, launch)
	case "windows":
		return applyFile(archive, exe, winExe, ".old", unzipTo, launch)
	case "linux":
		return applyFile(archive, exe, linuxExe, "", untarTo, launch)
	}
	return fmt.Errorf("updating is not supported on %s", goos)
}

// CleanupOld removes what an update left next to the program (the Windows one cannot delete itself while it runs).
func CleanupOld() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	_ = os.Remove(exe + ".old")
}

// bundleOf is the .app directory around the executable "<bundle>/Contents/MacOS/<name>".
func bundleOf(exe string) (string, error) {
	macos := filepath.Dir(exe)
	contents := filepath.Dir(macos)
	bundle := filepath.Dir(contents)
	if filepath.Base(macos) != "MacOS" || filepath.Base(contents) != "Contents" || !strings.HasSuffix(bundle, ".app") {
		return "", errors.New("the program is not inside an application bundle")
	}
	return bundle, nil
}

func applyDarwin(archive, exe string, launch func(string) error) error {
	bundle, err := bundleOf(exe)
	if err != nil {
		return err
	}
	parent := filepath.Dir(bundle)
	stage, err := os.MkdirTemp(parent, ".linager-update-")
	if err != nil {
		return fmt.Errorf("the folder of the application is not writable: %w", err)
	}
	defer func() { _ = os.RemoveAll(stage) }()
	if err := unzipTo(archive, stage); err != nil {
		return err
	}
	fresh, err := findApp(stage)
	if err != nil {
		return err
	}
	// an ad hoc signature is part of the bundle; a damaged one would not start
	if out, err := exec.Command("codesign", "--verify", "--deep", "--strict", fresh).CombinedOutput(); err != nil {
		if _, lerr := exec.LookPath("codesign"); lerr == nil {
			why := strings.TrimSpace(string(out))
			if i := strings.LastIndex(why, ": "); i >= 0 {
				why = why[i+2:] // without the temporary path
			}
			return fmt.Errorf("the new application is not valid (%s)", why)
		}
	}
	if err := swapBundle(bundle, fresh); err != nil {
		return err
	}
	return launch(bundle)
}

// findApp is the single .app directory at the top of an unpacked archive.
func findApp(dir string) (string, error) {
	es, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range es {
		if e.IsDir() && strings.HasSuffix(e.Name(), ".app") {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", errors.New("the archive holds no application")
}

// swapBundle puts fresh in place of bundle; the old one is kept until the new one stands, and restored if it cannot.
func swapBundle(bundle, fresh string) error {
	old := bundle + ".old"
	_ = os.RemoveAll(old)
	if err := os.Rename(bundle, old); err != nil {
		return err
	}
	if err := os.Rename(fresh, bundle); err != nil {
		if rerr := os.Rename(old, bundle); rerr != nil {
			return fmt.Errorf("%w (and the old application could not be put back: %v)", err, rerr)
		}
		return err
	}
	// the running program may be deleted from under itself on macOS and Linux
	_ = os.RemoveAll(old)
	return nil
}

// applyFile replaces the single-file program. keepOld ".old" keeps the old file next to it (Windows cannot overwrite a running program but can rename it).
func applyFile(archive, exe, name, keepOld string, unpack func(archive, dest string) error, launch func(string) error) error {
	dir, err := os.MkdirTemp(filepath.Dir(exe), ".linager-update-")
	if err != nil {
		return fmt.Errorf("the folder of the program is not writable: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err := unpack(archive, dir); err != nil {
		return err
	}
	fresh := filepath.Join(dir, name)
	fi, err := os.Stat(fresh)
	if err != nil || !fi.Mode().IsRegular() {
		return errors.New("the archive holds no program")
	}
	if err := os.Chmod(fresh, 0o755); err != nil {
		return err
	}
	if keepOld != "" {
		old := exe + keepOld
		_ = os.Remove(old)
		if err := os.Rename(exe, old); err != nil {
			return err
		}
		if err := os.Rename(fresh, exe); err != nil {
			if rerr := os.Rename(old, exe); rerr != nil {
				return fmt.Errorf("%w (and the old program could not be put back: %v)", err, rerr)
			}
			return err
		}
	} else if err := os.Rename(fresh, exe); err != nil {
		return err
	}
	return launch(exe)
}

// inside says whether the path stays under dest once cleaned (a defence against "../" names).
func inside(dest, p string) bool {
	rel, err := filepath.Rel(dest, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// safeLink says whether a symlink created at path points inside dest.
func safeLink(dest, path, target string) bool {
	if filepath.IsAbs(target) {
		return false
	}
	return inside(dest, filepath.Join(filepath.Dir(path), target))
}

func unzipTo(archive, dest string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer func() { _ = zr.Close() }()
	var total int64
	for _, f := range zr.File {
		// the extended attributes that macOS tools put in a zip as extra files would break the signature of a bundle
		if strings.HasPrefix(f.Name, "__MACOSX/") || strings.HasPrefix(filepath.Base(f.Name), "._") {
			continue
		}
		p := filepath.Join(dest, filepath.FromSlash(f.Name))
		if !inside(dest, p) {
			return fmt.Errorf("the archive has a path outside its folder: %q", f.Name)
		}
		mode := f.Mode()
		switch {
		case f.FileInfo().IsDir():
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
		case mode&os.ModeSymlink != 0:
			rc, err := f.Open()
			if err != nil {
				return err
			}
			target, err := io.ReadAll(io.LimitReader(rc, 4096))
			_ = rc.Close()
			if err != nil {
				return err
			}
			if !safeLink(dest, p, string(target)) {
				return fmt.Errorf("the archive has a link out of its folder: %q", f.Name)
			}
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(string(target), p); err != nil {
				return err
			}
		case mode.IsRegular():
			if total += int64(f.UncompressedSize64); total > maxUnpacked {
				return errors.New("the archive is too big")
			}
			rc, err := f.Open()
			if err != nil {
				return err
			}
			err = writeFile(p, rc, mode.Perm())
			_ = rc.Close()
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("the archive has an unexpected entry: %q", f.Name)
		}
	}
	return nil
}

func untarTo(archive, dest string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	var total int64
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		p := filepath.Join(dest, filepath.FromSlash(h.Name))
		if !inside(dest, p) {
			return fmt.Errorf("the archive has a path outside its folder: %q", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if !safeLink(dest, p, h.Linkname) {
				return fmt.Errorf("the archive has a link out of its folder: %q", h.Name)
			}
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(h.Linkname, p); err != nil {
				return err
			}
		case tar.TypeReg:
			if total += h.Size; total > maxUnpacked {
				return errors.New("the archive is too big")
			}
			if err := writeFile(p, tr, os.FileMode(h.Mode).Perm()); err != nil {
				return err
			}
		default:
			return fmt.Errorf("the archive has an unexpected entry: %q", h.Name)
		}
	}
}

func writeFile(p string, r io.Reader, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm|0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, io.LimitReader(r, maxUnpacked+1)); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
