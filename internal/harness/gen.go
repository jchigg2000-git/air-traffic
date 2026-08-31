// Package harness generates synthetic PII-seeded traffic, drives it through
// the inference gateway, scores the outcome against ground truth, and feeds
// the flywheel. It lives in the control plane as a *client* of the gateway —
// a traffic generator, never a request-path component; production inference
// does not traverse this package.
package harness

import (
	"fmt"
	"math/rand"

	"github.com/jchigg2000-git/air-traffic/internal/model"
)

// genItem is one generated request with its ground truth.
type genItem struct {
	Template string
	Content  string
	Truth    []model.TruthSpan
	Traps    []model.TruthSpan
	Straddle bool
	Replay   bool
}

// slot names used in templates
const (
	slotSSN     = "SSN"
	slotEmail   = "EMAIL"
	slotPhone   = "PHONE"
	slotCC      = "CREDIT_CARD"
	slotMRN     = "MRN"
	slotIP      = "IP"
	slotIBAN    = "IBAN"
	slotDOB     = "DOB"
	slotName    = "PERSON_NAME"
	slotAddress = "ADDRESS"
	slotTrap    = "TRAP"
)

// template is alternating literal / slot parts; slots become truth spans.
type template struct {
	name         string
	parts        []part
	presidioOnly bool // carries name/address slots regex can't catch
	trap         bool
}

type part struct {
	lit  string
	slot string
}

func lit(s string) part  { return part{lit: s} }
func slot(s string) part { return part{slot: s} }

func templates() []template {
	return []template{
		{name: "patient_intake", parts: []part{
			lit("Summarize this patient intake: MRN "), slot(slotMRN),
			lit(", SSN "), slot(slotSSN), lit(", DOB "), slot(slotDOB),
			lit(". Contact "), slot(slotEmail), lit(" with the care plan."),
		}},
		{name: "support_ticket", parts: []part{
			lit("Draft a reply to this ticket: customer at "), slot(slotEmail),
			lit(" (callback "), slot(slotPhone), lit(") reports a double charge on card "), slot(slotCC), lit("."),
		}},
		{name: "wire_transfer", parts: []part{
			lit("Explain this wire rejection: destination "), slot(slotIBAN),
			lit(", contact "), slot(slotPhone), lit(", SSN on file "), slot(slotSSN), lit("."),
		}},
		{name: "infra_note", parts: []part{
			lit("Review this runbook step: EHR host "), slot(slotIP),
			lit(" pages the on-call at "), slot(slotPhone), lit(" after three failures."),
		}},
		{name: "claims_review", parts: []part{
			lit("Check this claim summary for errors: member SSN "), slot(slotSSN),
			lit(", card "), slot(slotCC), lit(", reach at "), slot(slotEmail), lit("."),
		}},
		{name: "referral_letter", presidioOnly: true, parts: []part{
			lit("Write a referral letter for "), slot(slotName),
			lit(" living at "), slot(slotAddress),
			lit(". Include MRN "), slot(slotMRN), lit("."),
		}},
		{name: "discharge_note", presidioOnly: true, parts: []part{
			lit("Summarize the discharge for "), slot(slotName),
			lit(" (DOB "), slot(slotDOB), lit(", SSN "), slot(slotSSN), lit(")."),
		}},
		{name: "commerce_trap", trap: true, parts: []part{
			lit("Where is order "), slot(slotTrap),
			lit("? The customer emailed from "), slot(slotEmail), lit(" yesterday."),
		}},
		{name: "release_trap", trap: true, parts: []part{
			lit("Changelog: bump to "), slot(slotTrap),
			lit(" and rotate the pager to "), slot(slotPhone), lit("."),
		}},
	}
}

type generator struct{ r *rand.Rand }

// generate builds count deterministic items from seed. Replay entries from
// the promoted corpus are mixed in at replayPercent — the accumulated
// regression set (flywheel step 1).
func generate(cfg model.HarnessRunConfig, corpus []model.CorpusEntry) []genItem {
	g := &generator{r: rand.New(rand.NewSource(cfg.Seed))}
	tmpls := templates()
	var usable []template
	for _, t := range tmpls {
		if t.trap && !cfg.IncludeTraps {
			continue
		}
		if t.presidioOnly && !cfg.IncludePresidioOnly {
			continue
		}
		usable = append(usable, t)
	}
	items := make([]genItem, 0, cfg.Count)
	for i := 0; i < cfg.Count; i++ {
		if cfg.ReplayPercent > 0 && len(corpus) > 0 && g.r.Intn(100) < cfg.ReplayPercent {
			e := corpus[g.r.Intn(len(corpus))]
			items = append(items, genItem{Template: "corpus_replay", Content: e.Text, Truth: e.Truth, Replay: true})
			continue
		}
		t := usable[g.r.Intn(len(usable))]
		item := g.fill(t)
		if cfg.IncludeStraddle && !t.trap && len(item.Truth) > 0 && g.r.Intn(5) == 0 {
			item.Straddle = true
		}
		items = append(items, item)
	}
	return items
}

func (g *generator) fill(t template) genItem {
	item := genItem{Template: t.name}
	content := ""
	for _, p := range t.parts {
		if p.slot == "" {
			content += p.lit
			continue
		}
		val, typ := g.value(p.slot)
		sp := model.TruthSpan{Start: len(content), End: len(content) + len(val), Type: typ, Value: val}
		if p.slot == slotTrap {
			item.Traps = append(item.Traps, sp)
		} else {
			item.Truth = append(item.Truth, sp)
		}
		content += val
	}
	item.Content = content
	return item
}

// value returns a synthetic value for the slot. Some variants are deliberate
// v0 detector misses (bare SSN/MRN) — they exist to feed the flywheel.
// NB: the parameter must not be named after a slot constant — an earlier
// version shadowed slotName and turned every trap into a person name.
func (g *generator) value(slotKey string) (string, string) {
	r := g.r
	switch slotKey {
	case slotSSN:
		a, b, c := 100+r.Intn(665), 10+r.Intn(89), 1000+r.Intn(8999)
		switch r.Intn(4) {
		case 0:
			return fmt.Sprintf("%03d %02d %04d", a, b, c), slotSSN
		case 1:
			return fmt.Sprintf("%03d%02d%04d", a, b, c), slotSSN // bare → miss fodder
		default:
			return fmt.Sprintf("%03d-%02d-%04d", a, b, c), slotSSN
		}
	case slotEmail:
		users := []string{"jane.doe", "p.kim", "alex.rivera", "m.chen", "sam.okafor", "billing+acct"}
		domains := []string{"example.org", "clinic-mail.co.uk", "example.net", "hospital-demo.io"}
		return users[r.Intn(len(users))] + fmt.Sprintf("%d", r.Intn(90)+10) + "@" + domains[r.Intn(len(domains))], slotEmail
	case slotPhone:
		a, b, c := 200+r.Intn(700), 200+r.Intn(700), 1000+r.Intn(8999)
		switch r.Intn(3) {
		case 0:
			return fmt.Sprintf("(%03d) %03d-%04d", a, b, c), slotPhone
		case 1:
			return fmt.Sprintf("+1%03d%03d%04d", a, b, c), slotPhone
		default:
			return fmt.Sprintf("%03d-%03d-%04d", a, b, c), slotPhone
		}
	case slotCC:
		return g.luhnCard(), slotCC
	case slotMRN:
		if r.Intn(3) == 0 { // bare with context words → miss fodder
			return fmt.Sprintf("%08d", 10000000+r.Intn(89999999)), slotMRN
		}
		return fmt.Sprintf("MRN-%07d", 1000000+r.Intn(8999999)), slotMRN
	case slotIP:
		return fmt.Sprintf("10.%d.%d.%d", r.Intn(255), r.Intn(255), 1+r.Intn(254)), slotIP
	case slotIBAN:
		return g.iban(), slotIBAN
	case slotDOB:
		return fmt.Sprintf("%02d/%02d/%d", 1+r.Intn(12), 1+r.Intn(28), 1940+r.Intn(65)), slotDOB
	case slotName:
		first := []string{"John", "Maria", "Wei", "Aisha", "Diego", "Ingrid", "Kofi", "Yuki"}
		last := []string{"Smith", "Alvarez", "Chen", "Okafor", "Larsen", "Tanaka", "Mensah", "Novak"}
		return first[r.Intn(len(first))] + " " + last[r.Intn(len(last))], slotName
	case slotAddress:
		streets := []string{"Evergreen Terrace", "Maple Avenue", "Cedar Lane", "Willow Court"}
		cities := []string{"Springfield", "Riverton", "Lakewood", "Fairview"}
		return fmt.Sprintf("%d %s, %s", 100+r.Intn(900), streets[r.Intn(len(streets))], cities[r.Intn(len(cities))]), slotAddress
	case slotTrap:
		switch r.Intn(4) {
		case 0:
			return fmt.Sprintf("ORD-%03d-%02d-%04d", 100+r.Intn(899), 10+r.Intn(89), 1000+r.Intn(8999)), slotTrap
		case 1:
			return fmt.Sprintf("%09d", 100000000+r.Intn(899999999)), slotTrap // bare order id
		case 2:
			return fmt.Sprintf("v%d.%d.%d", 1+r.Intn(9), r.Intn(20), r.Intn(20)), slotTrap
		default:
			return g.invalidLuhn(), slotTrap
		}
	}
	return "", slotKey
}

// luhnCard generates a Luhn-valid 16-digit card from a real-looking prefix.
func (g *generator) luhnCard() string {
	prefixes := []string{"4", "51", "37"}
	p := prefixes[g.r.Intn(len(prefixes))]
	digits := p
	for len(digits) < 15 {
		digits += fmt.Sprintf("%d", g.r.Intn(10))
	}
	sum := 0
	double := true
	for i := len(digits) - 1; i >= 0; i-- {
		d := int(digits[i] - '0')
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	check := (10 - sum%10) % 10
	full := digits + fmt.Sprintf("%d", check)
	if g.r.Intn(2) == 0 {
		return full[:4] + " " + full[4:8] + " " + full[8:12] + " " + full[12:]
	}
	return full
}

func (g *generator) invalidLuhn() string {
	card := g.luhnCard()
	last := card[len(card)-1]
	bump := byte('0') + (last-'0'+1)%10
	return card[:len(card)-1] + string(bump)
}

// iban generates a checksum-valid German IBAN (DE + 2 check digits + 18 BBAN
// digits) so the mod-97 validator accepts it.
func (g *generator) iban() string {
	bban := ""
	for i := 0; i < 18; i++ {
		bban += fmt.Sprintf("%d", g.r.Intn(10))
	}
	// check digits: 98 - mod97(BBAN + "DE00" numeric form)
	rearranged := bban + "131400" // D=13, E=14, 00
	rem := 0
	for _, c := range rearranged {
		rem = (rem*10 + int(c-'0')) % 97
	}
	return fmt.Sprintf("DE%02d%s", 98-rem, bban)
}
