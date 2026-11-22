package slicer

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func archive(t *testing.T, xml string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	f, err := z.Create("Metadata/model_settings.config")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Write([]byte(xml)); err != nil {
		t.Fatal(err)
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

const object = `<object id="2"><metadata key="name" value="Synthetic bracket"/></object>`
const instanceXML = `<model_instance><metadata key="object_id" value="2"/><metadata key="instance_id" value="0"/></model_instance>`

func Test3MFLayoutsHaveIndependentPlatesAndUnknownEstimates(t *testing.T) {
	raw := `<config>` + object + `<plate><metadata key="plater_id" value="1"/>` + instanceXML + `</plate><plate><metadata key="plater_id" value="2"/>` + instanceXML + `</plate></config>`
	plates, err := Parse3MF(archive(t, raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(plates) != 2 || len(plates[0].Items) != 1 || plates[0].Items[0].Planned != 1 || plates[0].EstimatedTimeMinutes != nil || len(plates[0].Materials) != 0 {
		t.Fatalf("invented or flattened layout: %+v", plates)
	}
}
func Test3MFRejectsMalformedAmbiguousOrUnsupportedMetadata(t *testing.T) {
	valid := `<config>` + object + `<plate><metadata key="plater_id" value="1"/>` + instanceXML + `</plate></config>`
	for _, raw := range []string{`bad`, `<config/>`, strings.ReplaceAll(valid, `value="2"`, `value="missing"`), strings.Replace(valid, instanceXML, instanceXML+instanceXML, 1), strings.Replace(valid, `value="0"`, `value="-1"`, 1), strings.Repeat("x", (2<<20)+1)} {
		if _, err := Parse3MF(archive(t, raw)); err == nil {
			t.Fatal("accepted invalid metadata")
		}
	}
	if _, err := Parse3MF([]byte("bad zip")); err == nil {
		t.Fatal("accepted corrupt zip")
	}
}
