package epub

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/checked"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func faustExcerpt(t *testing.T) Extraction {
	t.Helper()
	data := fixtureDirectory(t, "testfixtures/faust-excerpt")
	extraction, err := ExtractV2(data)
	require.NoError(t, err)
	return extraction
}

func unitByManifest(t *testing.T, book ExtractedBook, manifest string) ExtractedUnit {
	t.Helper()
	for _, unit := range book.ExtractedUnits.Units {
		if unit.ManifestID == manifest {
			return unit
		}
	}
	t.Fatalf("no unit for manifest %q", manifest)
	return ExtractedUnit{}
}

func unitSpanText(unit ExtractedUnit, span domain.UnitRoleSpan) string {
	return string([]rune(unit.Text)[span.StartOffset:span.EndOffset])
}

func findSpan(t *testing.T, structure domain.SnapshotStructure, unit ExtractedUnit, role domain.SourceStructureRole, text string) domain.UnitRoleSpan {
	t.Helper()
	for _, span := range structure.RoleSpans {
		if span.UnitID != unit.ID || span.Role != role {
			continue
		}
		if unitSpanText(unit, span) == text {
			return span
		}
	}
	t.Fatalf("no %s span for %q in %q", role, text, unit.Text)
	return domain.UnitRoleSpan{}
}

func TestExtractV2FaustExcerpts(t *testing.T) {
	extraction := faustExcerpt(t)
	require.Equal(t, domain.ExtractedUnitsSchemaVersionV2, extraction.Book.ExtractedUnits.SchemaVersion)
	require.True(t, extraction.Structure.Recorded)
	assert.Equal(t, domain.StructureConventionSet, extraction.Structure.ConventionSet)
	require.NoError(t, extraction.Structure.Validate(domain.ExtractedUnitsSchemaVersionV2, extraction.Book.ExtractedUnits.Units))

	t.Run("Director speech is one 26-line group and the first sentence spans four lines", func(t *testing.T) {
		unit := unitByManifest(t, extraction.Book, "excerpt-0")
		groups := strings.Split(unit.Text, "\n\n")
		var speech string
		for _, group := range groups {
			if strings.HasPrefix(group, "Ihr beyden die ihr mir so oft,") {
				speech = group
			}
		}
		require.NotEmpty(t, speech)
		lines := strings.Split(speech, "\n")
		require.Len(t, lines, 26)
		assert.Equal(t, []string{
			"Ihr beyden die ihr mir so oft,",
			"In Noth und Trübsal, beygestanden,", //nolint:misspell // Goethe source text
			"Sagt was ihr wohl, in deutschen Landen,",
			"Von unsrer Unternehmung hofft?",
		}, lines[:4])
		assert.Equal(t, "Ich wünschte sehr der Menge zu behagen,", lines[4])
		assert.Equal(t, uint64(0), findHeadingReset(t, extraction.Structure, unit, "Vorspiel").Offset)

		entrance := findSpan(t, extraction.Structure, unit, domain.SourceStructureRoleDirection, "Director, Theaterdichter,\nlustige Person.")
		assert.Equal(t, "Director, Theaterdichter,\nlustige Person.", unitSpanText(unit, entrance))
		findSpan(t, extraction.Structure, unit, domain.SourceStructureRoleSpeaker, "Director.")
	})

	t.Run("Thule song keeps six four-line groups", func(t *testing.T) {
		unit := unitByManifest(t, extraction.Book, "excerpt-3")
		var stanzas []string
		for group := range strings.SplitSeq(unit.Text, "\n\n") {
			if len(strings.Split(group, "\n")) == 4 {
				stanzas = append(stanzas, group)
			}
		}
		require.Equal(t, []string{
			"Es war ein König in Thule\nGar treu bis an das Grab,\nDem sterbend seine Buhle\nEinen goldnen Becher gab.",
			"Es ging ihm nichts darüber,\nEr leert ihn jeden Schmaus;\nDie Augen gingen ihm über,\nSo oft er trank daraus.",
			"Und als er kam zu sterben,\nZählt’ er seine Städt’ im Reich,\nGönnt’ alles seinem Erben,\nDen Becher nicht zugleich.",
			"Er saß beym Königsmahle,\nDie Ritter um ihn her,\nAuf hohem Väter-Saale,\nDort auf dem Schloß am Meer.",
			"Dort stand der alte Zecher,\nTrank letzte Lebensgluth,\nUnd warf den heiligen Becher\nHinunter in die Fluth.",
			"Er sah ihn stürzen, trinken\nUnd sinken tief ins Meer,\nDie Augen thäten ihm sinken,\nTrank nie einen Tropfen mehr.",
		}, stanzas)
	})

	t.Run("correction markup contributes visible text only", func(t *testing.T) {
		unit := unitByManifest(t, extraction.Book, "excerpt-5")
		assert.Contains(t, unit.Text, "Der hatt’ ein — — —\nSo — es war, gefiel mir’s doch.")
		assert.Contains(t, unit.Text, "Halt’ er einen — — bereit,\nWenn er — — — nicht scheut.")
		for _, hidden := range []string{"ungeheures Loch", "groß", "rechten Pfropf", "das große Loch"} {
			assert.NotContains(t, unit.Text, hidden)
		}
	})

	t.Run("Valentin label stores a nested direction and attributes the following text", func(t *testing.T) {
		unit := unitByManifest(t, extraction.Book, "excerpt-4")
		speaker := findSpan(t, extraction.Structure, unit, domain.SourceStructureRoleSpeaker, "Valentin fällt.")
		direction := findSpan(t, extraction.Structure, unit, domain.SourceStructureRoleDirection, "fällt.")
		assert.LessOrEqual(t, speaker.StartOffset, direction.StartOffset)
		assert.GreaterOrEqual(t, speaker.EndOffset, direction.EndOffset)

		text := []rune(unit.Text)
		startRune, err := checked.Uint64FromInt(len(text) - len([]rune("O weh!")))
		require.NoError(t, err)
		start := startRune
		assert.Equal(t, domain.SourceAttribution{Kind: domain.AttributionSpeaker, Speaker: "Valentin"}, extraction.Structure.AttributionAt(unit, start, start+6))
		assert.Equal(t, domain.SourceAttribution{Kind: domain.AttributionStageDirection}, extraction.Structure.AttributionAt(unit, direction.StartOffset, direction.EndOffset))
	})

	t.Run("mixed-role paragraph stores the inline speaker span only", func(t *testing.T) {
		unit := unitByManifest(t, extraction.Book, "excerpt-2")
		findSpan(t, extraction.Structure, unit, domain.SourceStructureRoleSpeaker, "Mephistopheles")
		for _, span := range extraction.Structure.RoleSpans {
			if span.UnitID == unit.ID {
				assert.NotEqual(t, domain.SourceStructureRoleDirection, span.Role, "the encompassing direction class is contradictory and records no role")
			}
		}
	})

	t.Run("v1 keeps the old break behavior", func(t *testing.T) {
		v1, err := Extract(fixtureDirectory(t, "testfixtures/faust-excerpt"))
		require.NoError(t, err)
		unit := unitByManifest(t, v1, "excerpt-0")
		assert.Equal(t, domain.ExtractedUnitsSchemaVersionV1, v1.ExtractedUnits.SchemaVersion)
		assert.Contains(t, unit.Text,
			"Ihr beyden die ihr mir so oft,\n\nIn Noth und Trübsal, beygestanden,") //nolint:misspell // Goethe source text
	})
}

func findHeadingReset(t *testing.T, structure domain.SnapshotStructure, unit ExtractedUnit, heading string) domain.UnitHeadingReset {
	t.Helper()
	runes := []rune(unit.Text)
	for _, reset := range structure.HeadingResets {
		if reset.UnitID != unit.ID {
			continue
		}
		if strings.HasPrefix(string(runes[reset.Offset:]), heading) {
			return reset
		}
	}
	t.Fatalf("no heading reset for %q in %q", heading, unit.Text)
	return domain.UnitHeadingReset{}
}

func TestExtractV2MatchesV1ForNoBreakProse(t *testing.T) {
	directories := []string{
		"testdata",
		"testfixtures/epub2-clean",
		"testfixtures/epub3-edge-cases",
		"testfixtures/epub3-main-text",
		"testfixtures/epub3-reimport",
	}
	for _, directory := range directories {
		t.Run(directory, func(t *testing.T) {
			data := fixtureDirectory(t, directory)
			v1, err := Extract(data)
			require.NoError(t, err)
			v2, err := ExtractV2(data)
			require.NoError(t, err)

			assert.Equal(t, domain.ExtractedUnitsSchemaVersionV1, v1.ExtractedUnits.SchemaVersion, "the default writer stays v1")
			assert.Equal(t, domain.ExtractedUnitsSchemaVersionV2, v2.Book.ExtractedUnits.SchemaVersion)
			assert.Equal(t, v1.FullText, v2.Book.FullText, "plain blocks without <br/> keep today's text exactly")
			assert.Equal(t, v1.Title, v2.Book.Title)
			assert.Equal(t, v1.ExtractedUnits.Units, v2.Book.ExtractedUnits.Units)
			assert.True(t, v2.Structure.Recorded)
			require.NoError(t, v2.Structure.Validate(domain.ExtractedUnitsSchemaVersionV2, v2.Book.ExtractedUnits.Units))
		})
	}
}

func TestFaustExcerptFixtureProvenance(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testfixtures", "faust-excerpt.provenance.json"))
	require.NoError(t, err)
	var provenance struct {
		SourceSHA256  string `json:"source_sha256"`
		PackagePrefix string `json:"package_prefix"`
		Excerpts      []struct {
			Path string `json:"path"`
		} `json:"excerpts"`
	}
	require.NoError(t, json.Unmarshal(data, &provenance))
	assert.Equal(t, "93d5a2bd657dc0ffdb39dbff0ab0979cb1229be05e1efbf69ca08691b1430b01", provenance.SourceSHA256)
	assert.Equal(t, "OEBPS/8980964947130898904_21000-h-", provenance.PackagePrefix)
	require.NotEmpty(t, provenance.Excerpts)

	root := filepath.Join("testfixtures", "faust-excerpt")
	listed := make(map[string]bool, len(provenance.Excerpts))
	for _, excerpt := range provenance.Excerpts {
		listed[excerpt.Path] = true
		_, err := os.Stat(filepath.Join(root, filepath.FromSlash(excerpt.Path)))
		require.NoError(t, err, "listed excerpt is missing")
	}
	var unlisted, gutenberg []string
	require.NoError(t, filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := os.ReadFile(path) //nolint:gosec // path walks the committed fixture tree
		if err != nil {
			return err
		}
		if strings.Contains(strings.ToLower(string(content)), "gutenberg") {
			gutenberg = append(gutenberg, path)
		}
		if !strings.HasSuffix(path, ".htm.html") {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if !listed[filepath.ToSlash(relative)] {
			unlisted = append(unlisted, filepath.ToSlash(relative))
		}
		return nil
	}))
	assert.Empty(t, gutenberg, "the fixture must not embed Gutenberg header, licence or reference text")
	assert.Empty(t, unlisted, "every excerpt must be bound to the provenance record")
}

func TestExtractXHTMLV2BreakAndGroupContract(t *testing.T) {
	document, err := extractXHTMLV2(xhtmlFile(t, `<html xmlns="http://www.w3.org/1999/xhtml"><body>
		<h2>Erster Teil</h2><p class="subheader">Untertitel</p><hr/>
		<p>eins<br/>zwei<br/><br/>drei</p><p>vier</p>
	</body></html>`))
	require.NoError(t, err)
	assert.Equal(t, "Erster Teil\n\nUntertitel\n\neins\nzwei\n\ndrei\n\nvier", documentText(document.groups))
	require.Len(t, document.groups, 5)
	assert.Equal(t, []string{"Erster Teil", "Untertitel", "eins\nzwei", "drei", "vier"}, []string{
		document.groups[0].text, document.groups[1].text, document.groups[2].text, document.groups[3].text, document.groups[4].text,
	})
	assert.Equal(t, []string{
		document.groups[0].text, document.groups[1].text, document.groups[2].text, document.groups[3].text, document.groups[4].text,
	}, strings.Split(documentText(document.groups), "\n\n"), "recorded groups are exactly the text between group separators")
	assert.True(t, document.groups[0].heading)
	assert.False(t, document.groups[1].heading, "a two-part heading stays two groups; only the heading resets attribution")

	extractor := spineExtractor{structure: domain.SnapshotStructure{Recorded: true, ConventionSet: domain.StructureConventionSet}}
	require.NoError(t, extractor.recordUnitStructure("unit", document.groups))
	assert.Equal(t, []domain.UnitHeadingReset{{UnitID: "unit", Offset: 0}}, extractor.structure.HeadingResets)
}

func TestExtractXHTMLV2RoleConventions(t *testing.T) {
	document, err := extractXHTMLV2(xhtmlFile(t, `<html xmlns="http://www.w3.org/1999/xhtml"><body>
		<p class="speaker">Valentin <span class="sd">fällt.</span></p>
		<p class="entrance"><span class="person">Faust</span> tritt ein.</p>
		<p class="sdcenter">Nachdem es still ist, <span class="speaker">Mephistopheles</span> lacht.</p>
		<p class="speaker entrance">Widerspruch.</p>
		<p class="unknown">Nichts.</p>
		<p>Ein <span class="speaker">Sprecher</span> und <span class="sd">Richtung</span>.</p>
		<p>Ein <span class="speaker">Val<span>ent</span>in.</span> Ende.</p>
	</body></html>`))
	require.NoError(t, err)
	extractor := spineExtractor{structure: domain.SnapshotStructure{Recorded: true, ConventionSet: domain.StructureConventionSet}}
	require.NoError(t, extractor.recordUnitStructure("unit", document.groups))
	unit := ExtractedUnit{ID: "unit", Text: documentText(document.groups), EndOffset: uint64(len([]rune(documentText(document.groups))))}

	assert.Equal(t, "Valentin fällt.", unitSpanText(unit, findSpan(t, extractor.structure, unit, domain.SourceStructureRoleSpeaker, "Valentin fällt.")))
	assert.Equal(t, "fällt.", unitSpanText(unit, findSpan(t, extractor.structure, unit, domain.SourceStructureRoleDirection, "fällt.")))
	assert.Equal(t, "Faust tritt ein.", unitSpanText(unit, findSpan(t, extractor.structure, unit, domain.SourceStructureRoleDirection, "Faust tritt ein.")))
	assert.Equal(t, "Mephistopheles", unitSpanText(unit, findSpan(t, extractor.structure, unit, domain.SourceStructureRoleSpeaker, "Mephistopheles")))
	findSpan(t, extractor.structure, unit, domain.SourceStructureRoleSpeaker, "Sprecher")
	findSpan(t, extractor.structure, unit, domain.SourceStructureRoleDirection, "Richtung")
	findSpan(t, extractor.structure, unit, domain.SourceStructureRoleSpeaker, "Valentin.")
	for _, span := range extractor.structure.RoleSpans {
		assert.NotEqual(t, "Widerspruch.", unitSpanText(unit, span), "contradictory block classes establish no role")
		assert.NotEqual(t, "Nichts.", unitSpanText(unit, span), "unrecognized markup establishes no role")
	}
	require.NoError(t, extractor.structure.Validate(domain.ExtractedUnitsSchemaVersionV2, []ExtractedUnit{unit}))
}

func TestExtractXHTMLV2UnicodeAndVisibleText(t *testing.T) {
	document, err := extractXHTMLV2(xhtmlFile(t, `<html xmlns="http://www.w3.org/1999/xhtml"><body>
		<p>Grüße 👋 &amp; <span class="x-ebookmaker-pageno" title="[7]"><a id="Page_7"></a></span>Café — “/”</p>
		<p class="speaker">Zöe <span class="sd">lacht.</span></p>
		<p>Der hatt’ ein <ins class="correction" title="newer editions">— — —</ins></p>
	</body></html>`))
	require.NoError(t, err)
	const want = "Grüße 👋 & Café — “/”\n\nZöe lacht.\n\nDer hatt’ ein — — —"
	assert.Equal(t, want, documentText(document.groups))
	assert.NotContains(t, documentText(document.groups), "newer editions")

	extractor := spineExtractor{structure: domain.SnapshotStructure{Recorded: true, ConventionSet: domain.StructureConventionSet}}
	require.NoError(t, extractor.recordUnitStructure("unit", document.groups))
	unit := ExtractedUnit{ID: "unit", Text: want, EndOffset: uint64(len([]rune(want)))}
	assert.Equal(t, "Zöe lacht.", unitSpanText(unit, findSpan(t, extractor.structure, unit, domain.SourceStructureRoleSpeaker, "Zöe lacht.")))
	assert.Equal(t, "lacht.", unitSpanText(unit, findSpan(t, extractor.structure, unit, domain.SourceStructureRoleDirection, "lacht.")))
}

func TestExtractXHTMLV2MatchesV1WithoutBreaks(t *testing.T) {
	f := xhtmlFile(t, `<html xmlns="http://www.w3.org/1999/xhtml"><body><h1>Rock &amp; Roll</h1><p>Caf&eacute;&nbsp;&mdash;&nbsp;&ldquo;hello&rdquo;&hellip; &copy;</p><p>Ein <em>betontes</em> Wort.</p></body></html>`)
	v1, title, err := extractXHTML(f)
	require.NoError(t, err)
	document, err := extractXHTMLV2(f)
	require.NoError(t, err)
	assert.Equal(t, v1, documentText(document.groups), "plain blocks without <br/> keep today's text exactly")
	assert.Equal(t, title, document.title)
}

func TestExtractXHTMLV2RejectsMalformedDocument(t *testing.T) {
	_, err := extractXHTMLV2(xhtmlFile(t, `<html><body><p>unclosed</body></html>`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed XHTML")

	_, err = extractXHTMLV2(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "manifest resource is missing")
}
