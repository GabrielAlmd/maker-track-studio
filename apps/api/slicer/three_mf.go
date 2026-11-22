// Package slicer inspects bounded slicer metadata without extracting or executing files.
package slicer

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strconv"
)

var ErrUnsupported = errors.New("unsupported or invalid Bambu 3MF layout")

type metadata struct {
	Key   string `xml:"key,attr"`
	Value string `xml:"value,attr"`
}
type layoutObject struct {
	ID       string     `xml:"id,attr"`
	Metadata []metadata `xml:"metadata"`
}
type instance struct {
	Metadata []metadata `xml:"metadata"`
}
type layoutPlate struct {
	Metadata  []metadata `xml:"metadata"`
	Instances []instance `xml:"model_instance"`
}
type layout struct {
	XMLName xml.Name       `xml:"config"`
	Objects []layoutObject `xml:"object"`
	Plates  []layoutPlate  `xml:"plate"`
}
type Item struct {
	SourceModel string `json:"source_model"`
	Planned     int    `json:"planned"`
}
type Plate struct {
	SourceLabel          string `json:"source_label"`
	Items                []Item `json:"items"`
	EstimatedTimeMinutes *int   `json:"estimated_time_minutes"`
	Materials            []any  `json:"materials"`
}

func value(rows []metadata, key string) string {
	for _, r := range rows {
		if r.Key == key {
			return r.Value
		}
	}
	return ""
}

// Parse3MF supports model_settings.config layout only. Sliced weights/time are
// deliberately unknown here: project presets and geometry are not estimates.
func Parse3MF(data []byte) ([]Plate, error) {
	if len(data) > 32<<20 {
		return nil, ErrUnsupported
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(z.File) > 4096 {
		return nil, ErrUnsupported
	}
	var config *zip.File
	for _, f := range z.File {
		if f.Name == "Metadata/model_settings.config" {
			if config != nil {
				return nil, ErrUnsupported
			}
			config = f
		}
	}
	if config == nil || config.UncompressedSize64 > 2<<20 {
		return nil, ErrUnsupported
	}
	r, err := config.Open()
	if err != nil {
		return nil, ErrUnsupported
	}
	defer r.Close()
	raw, err := io.ReadAll(io.LimitReader(r, (2<<20)+1))
	if err != nil || len(raw) > 2<<20 {
		return nil, ErrUnsupported
	}
	var l layout
	if xml.Unmarshal(raw, &l) != nil || len(l.Plates) == 0 || len(l.Plates) > 100 {
		return nil, ErrUnsupported
	}
	names := map[string]string{}
	for _, obj := range l.Objects {
		if obj.ID == "" || names[obj.ID] != "" {
			return nil, ErrUnsupported
		}
		name := value(obj.Metadata, "name")
		if name == "" {
			name = "Object " + obj.ID
		}
		if len(name) > 1000 {
			return nil, ErrUnsupported
		}
		names[obj.ID] = name
	}
	plates := []Plate{}
	ids := map[string]bool{}
	for _, p := range l.Plates {
		id := value(p.Metadata, "plater_id")
		if id == "" || ids[id] || len(id) > 100 {
			return nil, ErrUnsupported
		}
		ids[id] = true
		if len(p.Instances) == 0 || len(p.Instances) > 100 {
			return nil, ErrUnsupported
		}
		plate := Plate{SourceLabel: "3MF plate " + id, Items: []Item{}, Materials: []any{}}
		seen := map[string]bool{}
		for _, i := range p.Instances {
			obj := value(i.Metadata, "object_id")
			index := value(i.Metadata, "instance_id")
			n, err := strconv.Atoi(index)
			key := obj + ":" + strconv.Itoa(n)
			if names[obj] == "" || err != nil || n < 0 || seen[key] {
				return nil, ErrUnsupported
			}
			seen[key] = true
			plate.Items = append(plate.Items, Item{SourceModel: names[obj], Planned: 1})
		}
		plates = append(plates, plate)
	}
	return plates, nil
}
