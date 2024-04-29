package azappinsights

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/microsoft/ApplicationInsights-Go/appinsights"
	"github.com/microsoft/ApplicationInsights-Go/appinsights/contracts"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/diode"
	"github.com/rs/zerolog/internal/cbor"
	"github.com/rs/zerolog/log"
	"io"
)

const defaultAppInsightsLevel = contracts.Information

type Config struct {
	InstrumentationKey string
}

type azureApplicationInsightsWriter struct {
	telemetryClient appinsights.TelemetryClient
}

func NewWriter(config Config) io.Writer {
	telemetryClient := appinsights.NewTelemetryClient(config.InstrumentationKey)

	w := &azureApplicationInsightsWriter{
		telemetryClient: telemetryClient,
	}

	return w
}

func NewAsyncWriter(config Config) io.WriteCloser {
	return diode.NewWriter(NewWriter(config), 20024, 0, func(missed int) {
		log.Error().Int("missed", missed).
			Msg("missed log entries flush to Azure AppInsights")
	})
}

func (a *azureApplicationInsightsWriter) Write(p []byte) (n int, err error) {
	var event map[string]interface{}
	lenP := len(p)

	p = cbor.DecodeIfBinaryToBytes(p)
	d := json.NewDecoder(bytes.NewReader(p))
	d.UseNumber()

	err = d.Decode(&event)
	if err != nil {
		return 0, err
	}

	appInsightsLevel := defaultAppInsightsLevel
	if l, ok := event[zerolog.LevelFieldName].(string); ok {
		appInsightsLevel = levelToAppInsightsLevel(l)
	}
	traceTelemetry := appinsights.NewTraceTelemetry("", appInsightsLevel)

	if event[zerolog.MessageFieldName] != nil {
		if m, ok := event[zerolog.MessageFieldName].(string); ok {
			traceTelemetry.Message = m
		}
	}

	for key, value := range event {
		switch key {
		case zerolog.LevelFieldName, zerolog.TimestampFieldName, zerolog.MessageFieldName:
			continue
		}

		switch v := value.(type) {
		case string:
			traceTelemetry.Properties[key] = v
		case json.Number:
			traceTelemetry.Properties[key] = fmt.Sprint(value)
		default:
			b, err := zerolog.InterfaceMarshalFunc(value)
			if err != nil {
				traceTelemetry.Properties[key] = fmt.Sprintf("[error: %v]", err)
				traceTelemetry.SeverityLevel = contracts.Critical
			} else {
				traceTelemetry.Properties[key] = string(b)
			}
		}
	}

	if t, ok := traceTelemetry.Properties["traceId"]; ok {
		traceTelemetry.Tags.Operation().SetId(t)
	}

	a.telemetryClient.Track(traceTelemetry)

	return lenP, nil
}

func levelToAppInsightsLevel(l string) contracts.SeverityLevel {
	level, _ := zerolog.ParseLevel(l)
	switch level {
	case zerolog.DebugLevel:
		return contracts.Verbose
	case zerolog.InfoLevel:
		return contracts.Information
	case zerolog.WarnLevel:
		return contracts.Warning
	case zerolog.ErrorLevel:
		return contracts.Error
	case zerolog.FatalLevel, zerolog.PanicLevel:
		return contracts.Critical
	case zerolog.NoLevel, zerolog.Disabled:
		return defaultAppInsightsLevel
	default:
		return defaultAppInsightsLevel
	}

}
