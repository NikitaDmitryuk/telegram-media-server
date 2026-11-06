package tvcompat

import "testing"

func TestUnspecifiedAspectRatioIsNotAnEncodingReason(t *testing.T) {
	for _, ratio := range []string{"", "N/A", "0:1", "1:1", "2:2"} {
		if sampleAspectRatioMismatch(ratio) {
			t.Fatalf("unspecified/square SAR %q treated as mismatch", ratio)
		}
		info := validMedia(t)
		info.Streams[0].SampleAspectRatio = ratio
		plan, err := planPreparation(info, true, defaultTVHeight, tvLevel)
		if err != nil || plan.videoEncode {
			t.Fatalf("SAR %q triggered preparation: %+v err=%v", ratio, plan, err)
		}
	}
	if !sampleAspectRatioMismatch("4:3") {
		t.Fatal("explicit non-square ratio ignored")
	}
}
