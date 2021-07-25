package cfg

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/scrapli/scrapligo/channel"

	"github.com/scrapli/scrapligo/logging"

	"github.com/scrapli/scrapligo/driver/base"
	"github.com/scrapli/scrapligo/driver/network"
)

const (
	filePromptNoisy = "noisy"
	filePromptQuiet = "quiet"
	filePromptAlert = "alert"
)

type iosxePatterns struct {
	bytesFreePattern      *regexp.Regexp
	filePromptModePattern *regexp.Regexp
	outputHeaderPattern   *regexp.Regexp
}

var iosxePatternsInstance *iosxePatterns

func getIOSXEPatterns() *iosxePatterns {
	if iosxePatternsInstance == nil {
		iosxePatternsInstance = &iosxePatterns{
			bytesFreePattern: regexp.MustCompile(
				`(?i)(?P<bytes_available>\d+)(?: bytes free)`,
			),
			filePromptModePattern: regexp.MustCompile(`(?i)(?:file prompt )(?P<prompt_mode>\w+)`),
			// sort of a bad name, but it matches python version --  used to find the version
			// string in the config so we can remove anything in front of it
			outputHeaderPattern:   regexp.MustCompile(`(?im)(^version \d+\.\d+$)`),
		}
	}

	return iosxePatternsInstance
}

type IOSXECfg struct {
	conn                           *network.Driver
	VersionPattern                 *regexp.Regexp
	Filesystem                     string
	filesystemSpaceAvailBufferPerc float32
	configCommandMap               map[string]string
	candidateConfigFilename        string
	replaceConfig                  bool
}

// NewIOSXECfg return a cfg instance setup for an Cisco IOSXE device.
func NewIOSXECfg(
	conn *network.Driver,
	options ...Option,
) (*Cfg, error) {
	options = append(
		[]Option{WithConfigSources([]string{"running", "startup"}), WithFilesystem("flash:")},
		options...)

	c, err := newCfg(conn, options...)
	if err != nil {
		return nil, err
	}

	c.Platform = &IOSXECfg{
		conn:           conn,
		VersionPattern: regexp.MustCompile(`(?i)\d+\.[a-z0-9\(\).]+`),
		configCommandMap: map[string]string{
			"running": "show running-config",
			"startup": "show startup-config",
		},
		filesystemSpaceAvailBufferPerc: 10.0,
	}

	err = setPlatformOptions(c.Platform, options...)
	if err != nil {
		return nil, err
	}

	return c, nil
}

func (p *IOSXECfg) ClearConfigSession() {
	p.candidateConfigFilename = ""
}

// GetVersion get the version from the device.
func (p *IOSXECfg) GetVersion() (string, []*base.Response, error) {
	versionResult, err := p.conn.SendCommand("show version | i Version")
	if err != nil {
		return "", nil, err
	}

	return p.VersionPattern.FindString(versionResult.Result), []*base.Response{versionResult}, nil
}

// GetConfig get the configuration of a source datastore from the device.
func (p *IOSXECfg) GetConfig(source string) (string, []*base.Response, error) {
	cmd, err := getConfigCommand(p.configCommandMap, source)
	if err != nil {
		return "", nil, err
	}

	configResult, err := p.conn.SendCommand(cmd)

	if err != nil {
		return "", nil, err
	}

	return configResult.Result, []*base.Response{configResult}, nil
}

func (p *IOSXECfg) cleanConfig(config string) string {
	patterns := getIOSXEPatterns()

	configSectionIndices := patterns.outputHeaderPattern.FindStringIndex(config)

	if len(configSectionIndices) == 0 {
		// didnt find the header pattern
		return config
	}

	if len(configSectionIndices) == 2 { //nolint:gomnd
		return config[configSectionIndices[0]:]
	}

	panic("stripping config header failed, this is a bug, provided config is wonky, or both...")
}

func (p *IOSXECfg) prepareConfigPayload(config string) string {
	tcslhStartFile := fmt.Sprintf(
		`puts [open "%s%s" w+] {`,
		p.Filesystem,
		p.candidateConfigFilename,
	)
	tclshEndFile := "}"

	return strings.Join([]string{tcslhStartFile, config, tclshEndFile}, "\n")
}

func (p *IOSXECfg) getFilesystemSpaceAvail() (int, error) {
	patterns := getIOSXEPatterns()

	filesystemSizeResult, err := p.conn.SendCommand(fmt.Sprintf("dir %s | i bytes", p.Filesystem))
	if err != nil {
		return -1, ErrFailedToDetermineDeviceState
	}

	bytesAvailMatch := patterns.bytesFreePattern.FindStringSubmatch(filesystemSizeResult.Result)

	bytesAvail := -1

	for i, name := range patterns.bytesFreePattern.SubexpNames() {
		if i != 0 && name == "bytes_available" {
			bytesAvail, err = strconv.Atoi(bytesAvailMatch[i])
			if err != nil {
				return -1, err
			}
		}
	}

	return bytesAvail, nil
}

func (p *IOSXECfg) isSpaceSufficient(filesystemBytesAvail int, config string) bool {
	return float32(
		filesystemBytesAvail,
	) >= float32(
		len(config),
	)/(p.filesystemSpaceAvailBufferPerc/100.0)+float32(
		len(config),
	)
}

// LoadConfig load a candidate configuration.
func (p *IOSXECfg) LoadConfig(
	config string,
	replace bool,
	options *OperationOptions,
) ([]*base.Response, error) {
	p.replaceConfig = replace

	var scrapliResponses []*base.Response

	if options.AutoClean {
		config = p.cleanConfig(config)
	}

	filesystemBytesAvail, err := p.getFilesystemSpaceAvail()
	if err != nil {
		return nil, err
	}

	spaceSufficient := p.isSpaceSufficient(filesystemBytesAvail, config)
	if !spaceSufficient {
		return nil, ErrInsufficientSpaceAvailable
	}

	if p.candidateConfigFilename == "" {
		p.candidateConfigFilename = fmt.Sprintf("scrapli_cfg_%d", time.Now().Unix())

		logging.LogDebug(
			FormatLogMessage(
				p.conn,
				"debug",
				fmt.Sprintf(
					"candidate configuration filename name will be %s",
					p.candidateConfigFilename,
				),
			),
		)
	}

	config = p.prepareConfigPayload(config)

	originalReturnChar := p.conn.CommsReturnChar
	tclCommsReturnChar := "\r"

	err = p.conn.AcquirePriv("tclsh")
	if err != nil {
		return nil, err
	}

	p.conn.Channel.CommsReturnChar = &tclCommsReturnChar

	r, err := p.conn.SendConfig(config, base.WithDesiredPrivilegeLevel("tclsh"))
	if err != nil {
		return nil, err
	}

	scrapliResponses = append(scrapliResponses, r)

	err = p.conn.AcquirePriv(p.conn.DefaultDesiredPriv)
	if err != nil {
		return scrapliResponses, err
	}

	p.conn.Channel.CommsReturnChar = &originalReturnChar

	return scrapliResponses, nil
}

func (p *IOSXECfg) determineFilePromptMode() (string, error) {
	r, err := p.conn.SendCommand("show run | i file prompt")
	if err != nil {
		return "", err
	}

	patterns := getIOSXEPatterns()

	filePromptMatch := patterns.filePromptModePattern.FindString(r.Result)

	if filePromptMatch == "" {
		return filePromptAlert, nil
	}

	if strings.Contains(filePromptMatch, filePromptNoisy) {
		return filePromptNoisy, nil
	}

	return filePromptQuiet, nil
}

// AbortConfig abort the loaded candidate configuration.
func (p *IOSXECfg) AbortConfig() ([]*base.Response, error) {
	var scrapliResponses []*base.Response

	r, err := p.deleteCandidateConfigFile()

	scrapliResponses = append(scrapliResponses, r)

	return scrapliResponses, err
}

func (p *IOSXECfg) commitConfigMerge() (*base.Response, error) {
	filePromptMode, err := p.determineFilePromptMode()
	if err != nil {
		return nil, err
	}

	var mergeEvents []*channel.SendInteractiveEvent

	if filePromptMode == filePromptAlert {
		mergeEvents = []*channel.SendInteractiveEvent{
			{
				ChannelInput: fmt.Sprintf(
					"copy %s%s running-config",
					p.Filesystem,
					p.candidateConfigFilename,
				),
				ChannelResponse: "Destination filename",
				HideInput:       false,
			},
			{
				ChannelInput:    "",
				ChannelResponse: "",
				HideInput:       false,
			},
		}
	} else if filePromptMode == filePromptNoisy {
		mergeEvents = []*channel.SendInteractiveEvent{
			{
				ChannelInput: fmt.Sprintf(
					"copy %s%s running-config", p.Filesystem, p.candidateConfigFilename),
				ChannelResponse: "Source filename",
				HideInput:       false,
			},
			{
				ChannelInput:    "",
				ChannelResponse: "Destination filename",
				HideInput:       false,
			},
			{
				ChannelInput:    "",
				ChannelResponse: "",
				HideInput:       false,
			},
		}
	} else {
		mergeEvents = []*channel.SendInteractiveEvent{
			{
				ChannelInput: fmt.Sprintf(
					"copy %s%s running-config", p.Filesystem, p.candidateConfigFilename),
				ChannelResponse: "",
				HideInput:       false,
			},
		}
	}

	return p.conn.SendInteractive(mergeEvents)
}

// SaveConfig writes running config to startup config.
func (p *IOSXECfg) SaveConfig() (*base.Response, error) {
	filePromptMode, err := p.determineFilePromptMode()
	if err != nil {
		return nil, err
	}

	var saveEvents []*channel.SendInteractiveEvent

	if filePromptMode == filePromptAlert {
		saveEvents = []*channel.SendInteractiveEvent{
			{
				ChannelInput:    "copy running-config startup-config",
				ChannelResponse: "Destination filename",
				HideInput:       false,
			},
			{
				ChannelInput:    "",
				ChannelResponse: "",
				HideInput:       false,
			},
		}
	} else if filePromptMode == filePromptNoisy {
		saveEvents = []*channel.SendInteractiveEvent{
			{
				ChannelInput:    "copy running-config startup-config",
				ChannelResponse: "Source filename",
				HideInput:       false,
			},
			{
				ChannelInput:    "",
				ChannelResponse: "Destination filename",
				HideInput:       false,
			},
			{
				ChannelInput:    "",
				ChannelResponse: "",
				HideInput:       false,
			},
		}
	} else {
		saveEvents = []*channel.SendInteractiveEvent{
			{
				ChannelInput:    "copy running-config startup-config",
				ChannelResponse: "",
				HideInput:       false,
			},
		}
	}

	return p.conn.SendInteractive(saveEvents)
}

func (p *IOSXECfg) deleteCandidateConfigFile() (*base.Response, error) {
	filePromptMode, err := p.determineFilePromptMode()
	if err != nil {
		return nil, err
	}

	var saveEvents []*channel.SendInteractiveEvent

	if filePromptMode == filePromptAlert || filePromptMode == filePromptNoisy {
		saveEvents = []*channel.SendInteractiveEvent{
			{
				ChannelInput: fmt.Sprintf(
					"delete %s%s",
					p.Filesystem,
					p.candidateConfigFilename,
				),
				ChannelResponse: "Delete filename",
				HideInput:       false,
			},
			{
				ChannelInput:    "",
				ChannelResponse: "[confirm]",
				HideInput:       false,
			},
			{
				ChannelInput:    "",
				ChannelResponse: "",
				HideInput:       false,
			},
		}
	} else {
		saveEvents = []*channel.SendInteractiveEvent{
			{
				ChannelInput:    fmt.Sprintf("delete %s%s", p.Filesystem, p.candidateConfigFilename),
				ChannelResponse: "[confirm]",
				HideInput:       false,
			},
			{
				ChannelInput:    "",
				ChannelResponse: "",
				HideInput:       false,
			},
		}
	}

	return p.conn.SendInteractive(saveEvents)
}

// CommitConfig commit the loaded candidate configuration.
func (p *IOSXECfg) CommitConfig(source string) ([]*base.Response, error) {
	var scrapliResponses []*base.Response

	var commitResult *base.Response

	var err error

	if p.replaceConfig {
		commitResult, err = p.conn.SendCommand(
			fmt.Sprintf("configure replace %s%s force", p.Filesystem, p.candidateConfigFilename),
		)
	} else {
		commitResult, err = p.commitConfigMerge()
	}

	scrapliResponses = append(scrapliResponses, commitResult)

	if err != nil {
		return scrapliResponses, err
	}

	saveResult, err := p.SaveConfig()

	scrapliResponses = append(scrapliResponses, saveResult)

	if err != nil {
		return scrapliResponses, err
	}

	cleanupResult, err := p.deleteCandidateConfigFile()

	scrapliResponses = append(scrapliResponses, cleanupResult)

	if err != nil {
		return scrapliResponses, err
	}

	return nil, nil
}

// DiffConfig diff the candidate configuration against a source config.
func (p *IOSXECfg) DiffConfig(
	source, candidateConfig string,
) (responses []*base.Response,
	normalizedSourceConfig,
	normalizedCandidateConfig,
	deviceDiff string, err error) {
	return nil, "", "", "", nil
}
