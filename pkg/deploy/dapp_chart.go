package deploy

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/flant/dapp/pkg/dapp"
	"github.com/flant/dapp/pkg/secret"
	"github.com/ghodss/yaml"
	"github.com/otiai10/copy"
	uuid "github.com/satori/go.uuid"
)

const (
	DefaultSecretValuesFile = "secret-values.yaml"
	SecretDirName           = "secret"
	DecodedSecretDirName    = "decoded-secret"
	MoreValuesDirName       = "more-values"
)

type DappChart struct {
	ChartDir string
	Values   []string
	Set      []string

	moreValuesCounter uint
}

func (chart *DappChart) SetGlobalAnnotation(name, value string) error {
	// TODO: https://github.com/flant/dapp/issues/1069
	return nil
}

func (chart *DappChart) SetValues(values map[string]interface{}) error {
	path := filepath.Join(chart.ChartDir, MoreValuesDirName, fmt.Sprintf("%d.yaml", chart.moreValuesCounter))
	err := os.MkdirAll(filepath.Dir(path), os.ModePerm)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(values)
	if err != nil {
		return fmt.Errorf("cannot marshal values %#v to yaml: %s", values, err)
	}

	err = ioutil.WriteFile(path, data, 0400)
	if err != nil {
		return fmt.Errorf("error writing values file %s: %s", path, data)
	}

	chart.Values = append(chart.Values, path)
	chart.moreValuesCounter++

	return nil
}

func (chart *DappChart) SetValuesSet(set string) error {
	chart.Set = append(chart.Set, set)
	return nil
}

func (chart *DappChart) SetValuesFile(path string) error {
	chart.Values = append(chart.Values, path)
	return nil
}

func (chart *DappChart) SetSecretValuesFile(path string, secret secret.Secret) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read secret values file %s: %s", path, err)
	}

	decodedData, err := decodeSecretValues(data, secret)
	if err != nil {
		return fmt.Errorf("cannot decode secret values file %s data: %s", path, err)
	}

	newPath := filepath.Join(chart.ChartDir, MoreValuesDirName, fmt.Sprintf("%d.yaml", chart.moreValuesCounter))

	err = os.MkdirAll(filepath.Dir(newPath), os.ModePerm)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(newPath, decodedData, 0400)
	if err != nil {
		return fmt.Errorf("cannot write decoded secret values file %s: %s", newPath, err)
	}

	chart.Values = append(chart.Values, newPath)
	chart.moreValuesCounter++

	return nil
}

func (chart *DappChart) Deploy(releaseName string, namespace string, opts HelmChartOptions) error {
	return DeployHelmChart(chart.ChartDir, releaseName, namespace, HelmChartOptions{
		Set:         append(chart.Set, opts.Set...),
		Values:      append(chart.Values, opts.Values...),
		KubeContext: opts.KubeContext,
		DryRun:      opts.DryRun,
		Debug:       opts.Debug,
	})
}

func (chart *DappChart) Render() error {
	return nil
}

func (chart *DappChart) Lint() error {
	return nil
}

type DappChartOptions struct {
	Secret secret.Secret
}

func GenerateDappChart(projectDir string, opts DappChartOptions) (*DappChart, error) {
	tmpChartPath := filepath.Join(dapp.GetTmpDir(), fmt.Sprintf("dapp-chart-%s", uuid.NewV4().String()))
	return PrepareDappChart(projectDir, tmpChartPath, opts)
}

func PrepareDappChart(projectDir string, targetDir string, opts DappChartOptions) (*DappChart, error) {
	dappChart := &DappChart{ChartDir: targetDir}

	projectHelmDir := filepath.Join(projectDir, ".helm")
	err := copy.Copy(projectHelmDir, targetDir)
	if err != nil {
		return nil, fmt.Errorf("unable to copy project helm dir %s into %s: %s", projectHelmDir, targetDir, err)
	}

	helpersTplPath := filepath.Join(targetDir, "templates/_dapp_helpers.tpl")
	f, err := os.Create(helpersTplPath)
	if err != nil {
		return nil, fmt.Errorf("unable to create %s: %s", helpersTplPath, err)
	}
	_, err = f.Write(DappChartHelpersTpl)
	if err != nil {
		return nil, fmt.Errorf("unable to write %s: %s", helpersTplPath, err)
	}

	defaultSecretValues := filepath.Join(projectDir, DefaultSecretValuesFile)
	if _, err := os.Stat(defaultSecretValues); !os.IsNotExist(err) {
		err := dappChart.SetSecretValuesFile(defaultSecretValues, opts.Secret)
		if err != nil {
			return nil, err
		}
	}

	secretDir := filepath.Join(projectDir, SecretDirName)
	if _, err := os.Stat(secretDir); !os.IsNotExist(err) {
		err := filepath.Walk(secretDir, func(path string, info os.FileInfo, accessErr error) error {
			if accessErr != nil {
				return fmt.Errorf("error accessing file %s: %s", path, accessErr)
			}

			if info.Mode().IsDir() {
				return nil
			}

			relativePath := strings.TrimPrefix(path, secretDir)
			newPath := filepath.Join(targetDir, DecodedSecretDirName, relativePath)

			if opts.Secret == nil {
				err := os.MkdirAll(filepath.Dir(newPath), os.ModePerm)
				if err != nil {
					return err
				}
				err = ioutil.WriteFile(newPath, []byte{}, 0400)
				if err != nil {
					return fmt.Errorf("unable to create decoded secret file %s: %s", newPath, err)
				}
				return nil
			}

			data, err := ioutil.ReadFile(path)
			if err != nil {
				return fmt.Errorf("error reading file %s: %s", path, err)
			}

			decodedData, err := decodeSecret([]byte(strings.TrimRightFunc(string(data), unicode.IsSpace)), opts.Secret)
			if err != nil {
				return fmt.Errorf("error decoding %s: %s", path, err)
			}

			err = os.MkdirAll(filepath.Dir(newPath), os.ModePerm)
			if err != nil {
				return err
			}
			err = ioutil.WriteFile(newPath, decodedData, 0400)
			if err != nil {
				return fmt.Errorf("error writing file %s: %s", newPath, err)
			}

			return nil
		})

		if err != nil {
			return nil, err
		}
	}

	return dappChart, nil
}

func decodeSecretValues(data []byte, secret secret.Secret) ([]byte, error) {
	return data, nil
}

func decodeSecret(data []byte, secret secret.Secret) ([]byte, error) {
	return data, nil
}