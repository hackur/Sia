package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"net/http"
	"path"
	"path/filepath"
	"runtime"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/types"

	"github.com/inconshreveable/go-update"
	"github.com/julienschmidt/httprouter"
	"github.com/kardianos/osext"
)

var errEmptyUpdateResponse = errors.New("API call to https://api.github.com/repos/NebulousLabs/Sia/releases/latest is returning an empty response")

// SiaConstants is a struct listing all of the constants in use.
type SiaConstants struct {
	GenesisTimestamp      types.Timestamp   `json:"genesistimestamp"`
	BlockSizeLimit        uint64            `json:"blocksizelimit"`
	BlockFrequency        types.BlockHeight `json:"blockfrequency"`
	TargetWindow          types.BlockHeight `json:"targetwindow"`
	MedianTimestampWindow uint64            `json:"mediantimestampwindow"`
	FutureThreshold       types.Timestamp   `json:"futurethreshold"`
	SiafundCount          types.Currency    `json:"siafundcount"`
	SiafundPortion        *big.Rat          `json:"siafundportion"`
	MaturityDelay         types.BlockHeight `json:"maturitydelay"`

	InitialCoinbase uint64 `json:"initialcoinbase"`
	MinimumCoinbase uint64 `json:"minimumcoinbase"`

	RootTarget types.Target `json:"roottarget"`
	RootDepth  types.Target `json:"rootdepth"`

	MaxAdjustmentUp   *big.Rat `json:"maxadjustmentup"`
	MaxAdjustmentDown *big.Rat `json:"maxadjustmentdown"`

	SiacoinPrecision types.Currency `json:"siacoinprecision"`
}

type DaemonVersion struct {
	Version string `json:"version"`
}

// UpdateInfo indicates whether an update is available, and to what
// version.
type UpdateInfo struct {
	Available bool   `json:"available"`
	Version   string `json:"version"`
}

// githubRelease represents some of the JSON returned by the GitHub release API
// endpoint. Only the fields relevant to updating are included.
type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name        string `json:"name"`
		DownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

const (
	// The developer key is used to sign updates and other important Sia-
	// related information.
	developerKey = `-----BEGIN PUBLIC KEY-----
MIIEIjANBgkqhkiG9w0BAQEFAAOCBA8AMIIECgKCBAEAsoQHOEU6s/EqMDtw5HvA
YPTUaBgnviMFbG3bMsRqSCD8ug4XJYh+Ik6WP0xgq+OPDehPiaXK8ghAtBiW1EJK
mBRwlABXAzREZg8wRfG4l8Zj6ckAPJOgLn0jobXy6/SCQ+jZSWh4Y8DYr+LA3Mn3
EOga7Jvhpc3fTZ232GBGJ1BobuNfRfYmwxSphv+T4vzIA3JUjVfa8pYZGIjh5XbJ
5M8Lef0Xa9eqr6lYm5kQoOIXeOW56ImqI2BKg/I9NGw9phSPbwaFfy1V2kfHp5Xy
DtKnyj/O9zDi+qUKjoIivnEoV+3DkioHUWv7Fpf7yx/9cPyckwvaBsTd9Cfp4uBx
qJ5Qyv69VZQiD6DikNwgzjGbIjiLwfTObhInKZUoYl48yzgkR80ja5TW0SoidNvO
4WTbWcLolOl522VarTs7wlgbq0Ad7yrNVnHzo447v2iT20ILH2oeAcZqvpcvRmTl
U6uKoaVmBH3D3Y19dPluOjK53BrqfQ5L8RFli2wEJktPsi5fUTd4UI9BgnUieuDz
S7h/VH9bv9ZVvyjpu/uVjdvaikT3zbIy9J6wS6uE5qPLPhI4B9HgbrQ03muDGpql
gZrMiL3GdYrBiqpIbaWHfM0eMWEK3ZScUdtCgUXMMrkvaUJ4g9wEgbONFVVOMIV+
YubIuzBFqug6WyxN/EAM/6Fss832AwVPcYM0NDTVGVdVplLMdN8YNjrYuaPngBCG
e8QaTWtHzLujyBIkVdAHqfkRS65jp7JLLMx7jUA74/E/v+0cNew3Y1p2gt3iQH8t
w93xn9IPUfQympc4h3KerP/Yn6P/qAh68jQkOiMMS+VbCq/BOn8Q3GbR+8rQ8dmk
qVoGA7XrPQ6bymKBTghk2Ek+ZjxrpAoj0xYoYyzWf0kuxeOT8kAjlLLmfQ8pm75S
QHLqH49FyfeETIU02rkw2oMOX/EYdJzZukHuouwbpKSElpRx+xTnaSemMJo+U7oX
xVjma3Zynh9w12abnFWkZKtrxwXv7FCSzb0UZmMWUqWzCS03Rrlur21jp4q2Wl71
Vt92xe5YbC/jbh386F1e/qGq6p+D1AmBynIpp/HE6fPsc9LWgJDDkREZcp7hthGW
IdYPeP3CesFHnsZMueZRib0i7lNUkBSRneO1y/C9poNv1vOeTCNEE0jvhp/XOJuc
yCQtrUSNALsvm7F+bnwP2F7K34k7MOlOgnTGqCqW+9WwBcjR44B0HI+YERCcRmJ8
krBuVo9OBMV0cYBWpjo3UI9j3lHESCYhLnCz7SPap7C1yORc2ydJh+qjKqdLBHom
t+JydcdJLbIG+kb3jB9QIIu5A4TlSGlHV6ewtxIWLS1473jEkITiVTt0Y5k+VLfW
bwIDAQAB
-----END PUBLIC KEY-----`
)

// fetchLatestRelease returns metadata about the most recent GitHub release.
func fetchLatestRelease() (githubRelease, error) {
	req, err := http.NewRequest("GET", "https://api.github.com/repos/NebulousLabs/Sia/releases/latest", nil)
	if err != nil {
		return githubRelease{}, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return githubRelease{}, err
	}
	defer resp.Body.Close()
	var release githubRelease
	err = json.NewDecoder(resp.Body).Decode(&release)
	if err != nil {
		return githubRelease{}, err
	}
	if release.TagName == "" && len(release.Assets) == 0 {
		return githubRelease{}, errEmptyUpdateResponse
	}
	return release, nil
}

// updateToRelease updates siad and siac to the release specified. siac is
// assumed to be in the same folder as siad.
func updateToRelease(release githubRelease) error {
	updateOpts := update.Options{
		Verifier: update.NewRSAVerifier(),
	}
	err := updateOpts.SetPublicKeyPEM([]byte(developerKey))
	if err != nil {
		// should never happen
		return err
	}

	binaryFolder, err := osext.ExecutableFolder()
	if err != nil {
		return err
	}

	// construct release filename
	releaseName := fmt.Sprintf("Sia-%s-%s-%s.zip", release.TagName, runtime.GOOS, runtime.GOARCH)

	// find release
	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == releaseName {
			downloadURL = asset.DownloadURL
			break
		}
	}
	if downloadURL == "" {
		return errors.New("couldn't find download URL for " + releaseName)
	}

	// download release archive
	resp, err := http.Get(downloadURL)
	if err != nil {
		return err
	}
	// release should be small enough to store in memory (<10 MiB); use
	// LimitReader to ensure we don't download more than 32 MiB
	content, err := ioutil.ReadAll(io.LimitReader(resp.Body, 1<<25))
	resp.Body.Close()
	if err != nil {
		return err
	}
	r := bytes.NewReader(content)
	z, err := zip.NewReader(r, r.Size())
	if err != nil {
		return err
	}

	// process zip, finding siad/siac binaries and signatures
	for _, binary := range []string{"siad", "siac"} {
		var binData io.ReadCloser
		var signature []byte
		var binaryName string // needed for TargetPath below
		for _, zf := range z.File {
			switch base := path.Base(zf.Name); base {
			case binary, binary + ".exe":
				binaryName = base
				binData, err = zf.Open()
				if err != nil {
					return err
				}
				defer binData.Close()
			case binary + ".sig", binary + ".exe.sig":
				sigFile, err := zf.Open()
				if err != nil {
					return err
				}
				defer sigFile.Close()
				signature, err = ioutil.ReadAll(sigFile)
				if err != nil {
					return err
				}
			}
		}
		if binData == nil {
			return errors.New("could not find " + binary + " binary")
		} else if signature == nil {
			return errors.New("could not find " + binary + " signature")
		}

		// apply update
		updateOpts.Signature = signature
		updateOpts.TargetMode = 0775 // executable
		updateOpts.TargetPath = filepath.Join(binaryFolder, binaryName)
		err = update.Apply(binData, updateOpts)
		if err != nil {
			return err
		}
	}

	return nil
}

// daemonUpdateHandlerGET handles the API call that checks for an update.
func (srv *Server) daemonUpdateHandlerGET(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	release, err := fetchLatestRelease()
	if err != nil {
		writeError(w, Error{"Failed to fetch latest release: " + err.Error()}, http.StatusInternalServerError)
		return
	}
	latestVersion := release.TagName[1:] // delete leading 'v'
	writeJSON(w, UpdateInfo{
		Available: build.VersionCmp(latestVersion, build.Version) > 0,
		Version:   latestVersion,
	})
}

// daemonUpdateHandlerPOST handles the API call that updates siad and siac.
// There is no safeguard to prevent "updating" to the same release, so callers
// should always check the latest version via daemonUpdateHandlerGET first.
// TODO: add support for specifying version to update to.
func (srv *Server) daemonUpdateHandlerPOST(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	release, err := fetchLatestRelease()
	if err != nil {
		writeError(w, Error{"Failed to fetch latest release: " + err.Error()}, http.StatusInternalServerError)
		return
	}
	err = updateToRelease(release)
	if err != nil {
		if rerr := update.RollbackError(err); rerr != nil {
			writeError(w, Error{"Serious error: Failed to rollback from bad update: " + rerr.Error()}, http.StatusInternalServerError)
		} else {
			writeError(w, Error{"Failed to apply update: " + err.Error()}, http.StatusInternalServerError)
		}
		return
	}
	writeSuccess(w)
}

// debugConstantsHandler prints a json file containing all of the constants.
func (srv *Server) daemonConstantsHandler(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	sc := SiaConstants{
		GenesisTimestamp:      types.GenesisTimestamp,
		BlockSizeLimit:        types.BlockSizeLimit,
		BlockFrequency:        types.BlockFrequency,
		TargetWindow:          types.TargetWindow,
		MedianTimestampWindow: types.MedianTimestampWindow,
		FutureThreshold:       types.FutureThreshold,
		SiafundCount:          types.SiafundCount,
		SiafundPortion:        types.SiafundPortion,
		MaturityDelay:         types.MaturityDelay,

		InitialCoinbase: types.InitialCoinbase,
		MinimumCoinbase: types.MinimumCoinbase,

		RootTarget: types.RootTarget,
		RootDepth:  types.RootDepth,

		MaxAdjustmentUp:   types.MaxAdjustmentUp,
		MaxAdjustmentDown: types.MaxAdjustmentDown,

		SiacoinPrecision: types.SiacoinPrecision,
	}

	writeJSON(w, sc)
}

// daemonVersionHandler handles the API call that requests the daemon's version.
func (srv *Server) daemonVersionHandler(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	writeJSON(w, DaemonVersion{Version: build.Version})
}

// daemonStopHandler handles the API call to stop the daemon cleanly.
func (srv *Server) daemonStopHandler(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	// can't write after we stop the server, so lie a bit.
	writeSuccess(w)

	// need to flush the response before shutting down the server
	f, ok := w.(http.Flusher)
	if !ok {
		panic("Server does not support flushing")
	}
	f.Flush()

	if err := srv.Close(); err != nil {
		build.Critical(err)
	}
}
