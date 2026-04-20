// Package lang provides localised strings for the two supported UI languages.
package lang

import (
	"bytes"
	"math"
	"strconv"
	"strings"
	"text/template"
)

// Language represents a supported UI language.
type Language string

const (
	//nolint:varnamelen // two-letter language codes are the established convention
	ES Language = "es"
	//nolint:varnamelen // two-letter language codes are the established convention
	EN Language = "en"
)

// Parse returns the Language for code, defaulting to Spanish.
func Parse(code string) Language {
	switch strings.ToLower(code) {
	case "en", "english":
		return EN
	default:
		return ES
	}
}

// knownIcons is the set of icon values that map to a named unit type.
var knownIcons = map[string]struct{}{ //nolint:gochecknoglobals // package-level lookup table, initialised once
	"HeavyTank":     {},
	"MediumTank":    {},
	"LightTank":     {},
	"TankDestroyer": {},
	"SPG":           {},
	"SPAA":          {},
	"Fighter":       {},
	"Bomber":        {},
	"Helicopter":    {},
}

// IsIdentifiableIcon reports whether the icon maps to a known unit type.
func IsIdentifiableIcon(icon string) bool {
	_, ok := knownIcons[icon]
	return ok
}

// IconName returns the singular unit label.
func (l Language) IconName(icon string) string {
	type pair [2]string // [es, en]

	names := map[string]pair{
		"HeavyTank":     {"Tanque pesado", "Heavy tank"},
		"MediumTank":    {"Tanque medio", "Medium tank"},
		"LightTank":     {"Tanque ligero", "Light tank"},
		"TankDestroyer": {"Cazatanques", "Tank destroyer"},
		"SPG":           {"Artillería", "SPG"},
		"SPAA":          {"Antiaéreo", "AA gun"},
		"Fighter":       {"Caza", "Fighter"},
		"Bomber":        {"Bombardero", "Bomber"},
		"Helicopter":    {"Helicóptero", "Helicopter"},
	}
	if pair, ok := names[icon]; ok {
		if l == EN {
			return pair[1]
		}

		return pair[0]
	}

	if l == EN {
		return "unit (" + icon + ")"
	}

	return "unidad (" + icon + ")"
}

// IconNamePlural returns the plural unit label.
func (l Language) IconNamePlural(icon string) string {
	type pair [2]string

	names := map[string]pair{
		"HeavyTank":     {"Tanques pesados", "Heavy tanks"},
		"MediumTank":    {"Tanques medios", "Medium tanks"},
		"LightTank":     {"Tanques ligeros", "Light tanks"},
		"TankDestroyer": {"Cazatanques", "Tank destroyers"},
		"SPG":           {"Artillerías", "SPGs"},
		"SPAA":          {"Antiaéreos", "AA guns"},
		"Fighter":       {"Cazas", "Fighters"},
		"Bomber":        {"Bombarderos", "Bombers"},
		"Helicopter":    {"Helicópteros", "Helicopters"},
	}
	if pair, ok := names[icon]; ok {
		if l == EN {
			return pair[1]
		}

		return pair[0]
	}

	if l == EN {
		return "units (" + icon + ")"
	}

	return "unidades (" + icon + ")"
}

// countEN returns the English word for count (2–10) or a digit string for larger values.
func countEN(count int) string {
	words := [11]string{"", "", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten"}
	if count >= 2 && count <= 10 {
		return words[count]
	}

	return strconv.Itoa(count)
}

// countES returns the Spanish word for count (2–10) or a digit string for larger values.
func countES(count int) string {
	words := [11]string{"", "", "dos", "tres", "cuatro", "cinco", "seis", "siete", "ocho", "nueve", "diez"}
	if count >= 2 && count <= 10 {
		return words[count]
	}

	return strconv.Itoa(count)
}

// Count returns the word for n (2–10) or a digit string for larger values.
func (l Language) Count(n int) string {
	if l == EN {
		return countEN(n)
	}

	return countES(n)
}

// compassPair returns the (Spanish, English) direction pair for a bearing in degrees.
func compassPair(bearing float64) (string, string) {
	switch {
	case bearing < 22.5 || bearing >= 337.5:
		return "norte", "north"
	case bearing < 67.5:
		return "noreste", "northeast"
	case bearing < 112.5:
		return "este", "east"
	case bearing < 157.5:
		return "sureste", "southeast"
	case bearing < 202.5:
		return "sur", "south"
	case bearing < 247.5:
		return "suroeste", "southwest"
	case bearing < 292.5:
		return "oeste", "west"
	default:
		return "noroeste", "northwest"
	}
}

// CompassDir converts a bearing in degrees (0=north, clockwise) to a compass label.
func (l Language) CompassDir(bearing float64) string {
	esDir, enDir := compassPair(bearing)
	if l == EN {
		return enDir
	}

	return esDir
}

// MovementDir returns the compass direction of (deltaX, deltaY), or "" when
// below minMovement (stationary noise filter).
func (l Language) MovementDir(deltaX, deltaY float64) string {
	const minMovement = 0.002
	if math.Hypot(deltaX, deltaY) < minMovement {
		return ""
	}

	// Positions use math convention (y increases north), so negate deltaY
	// to convert to screen convention before computing the compass bearing.
	bearing := math.Mod(math.Atan2(-deltaY, deltaX)*180/math.Pi+90+360, 360)

	return l.CompassDir(bearing)
}

// RelativeDir converts an angle in degrees (0=ahead, ±90=sides, ±180=behind)
// to a relative direction label.
func (l Language) RelativeDir(angle float64) string {
	type pair [2]string

	var dir pair

	switch {
	case angle > -22.5 && angle <= 22.5:
		dir = pair{"al frente", "ahead"}
	case angle > 22.5 && angle <= 112.5:
		dir = pair{"por la derecha", "to the right"}
	case angle > -112.5 && angle <= -22.5:
		dir = pair{"por la izquierda", "to the left"}
	default:
		dir = pair{"por la retaguardia", "from behind"}
	}

	if l == EN {
		return dir[1]
	}

	return dir[0]
}

// ClockPosition converts an angle in degrees (0=ahead, positive=right, ±180=behind)
// to a clock-position string such as "twelve o'clock" or "three o'clock".
func ClockPosition(angle float64) string {
	hours := [12]string{
		"twelve", "one", "two", "three", "four", "five",
		"six", "seven", "eight", "nine", "ten", "eleven",
	}
	// Normalise to [0, 360) then bucket into 30° slices.
	normalized := math.Mod(angle+360, 360)
	hour := int(math.Round(normalized/30)) % 12

	return hours[hour] + " o'clock"
}

// PromptRelativeDir returns the relative direction for use in LLM prompts.
// For English it returns a clock-position string ("twelve o'clock", "three o'clock"…)
// so the model does not need to translate compass labels.
// For other languages it falls back to the regular RelativeDir label.
func (l Language) PromptRelativeDir(angle float64) string {
	if l == EN {
		return ClockPosition(angle)
	}

	return l.RelativeDir(angle)
}

// FlankSide returns the side label ("left"/"right" or "izquierda"/"derecha")
// for angle < 0 = left.
func (l Language) FlankSide(angle float64) string {
	if angle < 0 {
		if l == EN {
			return "left"
		}

		return "izquierda"
	}

	if l == EN {
		return "right"
	}

	return "derecha"
}

// FlankAlert returns the flank alert message for the given side and unit name.
func (l Language) FlankAlert(side, icon string) string {
	if l == EN {
		return "Flank from the " + side + ", " + icon + "!"
	}

	return "¡Flanqueo por la " + side + ", " + icon + "!"
}

// ZonePressureAlert returns the capture-zone-under-pressure alert.
func (l Language) ZonePressureAlert(label string) string {
	if l == EN {
		return "Zone " + label + " under pressure!"
	}

	return "¡Zona " + label + " bajo presión!"
}

// ZoneEnemyAlert returns an alert naming the unit type contesting a zone.
func (l Language) ZoneEnemyAlert(label, unitName string) string {
	if l == EN {
		return "Enemy " + unitName + " at " + label + "!"
	}

	return "¡Enemigo " + unitName + " en " + label + "!"
}

// DetectedSuffix returns " detected" / " detectado(s)" for n units.
func (l Language) DetectedSuffix(n int) string {
	if l == EN {
		return " detected"
	}

	if n == 1 {
		return " detectado"
	}

	return " detectados"
}

// MovingLabel returns the trailing "moving <dir>" clause appended to grouped
// detection messages when all enemies share the same direction.
func (l Language) MovingLabel(dir string) string {
	if l == EN {
		return ", moving " + dir
	}

	return ", moviéndose al " + dir
}

// AtZoneLabel returns the trailing "near zone <label>" clause appended to grouped
// detection messages when all enemies share the same capture zone.
func (l Language) AtZoneLabel(label string) string {
	if l == EN {
		return ", near zone " + label
	}

	return ", cerca de la zona " + label
}

// promptData is the data passed to each system-prompt template.
type promptData struct {
	Callsign       string
	WindowSecs     int
	PreviousAlerts string // formatted "[N reports ago] ..." list; empty when no history
}

// systemPromptTemplates maps "mode:lang" to a compiled template.
// Templates are parsed once at init time; a malformed template panics early.
var systemPromptTemplates = map[string]*template.Template{ //nolint:gochecknoglobals // package-level lookup table, initialised once
	"warning:en": template.Must(template.New("warning:en").Parse(
		`You are Actual, a forward intelligence operator calling in to {{.Callsign}} over a tactical radio net.
You receive a {{.WindowSecs}}-second battlefield summary AND the last alerts you already transmitted.
Your job: track active threats and escalate or vary your language as situations develop. Do not repeat the same phrasing — evolve it.

Restrictions:
- No orders or imperatives directed at {{.Callsign}}
- No "shoot", "retreat", "reposition" or action verbs targeting me
- Maximum 15 words
- Always qualify tango with type: light tango, medium tango, heavy tango, tank destroyer, or SPAA
- If nothing changed since the last report, say nothing. Absolute silence.
- Only speak when there is a concrete tactical change: new visible enemy, movement toward your flanks, ally eliminated, contested zone. A stationary enemy that was already reported does not justify another report.
- Never use the exact same sentence as a previous alert
- Distance values in the data are in meters — use them directly (e.g. "350 meters", "120 meters"). Do not convert to qualitative terms.

Threat priority:
1. Flank or rear exposure from ally loss
2. Enemy on sustained approach — escalate urgency as distance closes
3. Stationary enemy with firing angle
4. Capture zone under pressure, no friendly coverage
5. Squad movement status

Urgency by distance for closing threats:
- dist > 300 meters → awareness tone: "contact, light tango closing right flank"
- dist 80–300 meters → elevated tone: "right flank medium tango still closing, 180 meters"
- dist < 80 meters → critical tone: "right flank critical — heavy tango at 60 meters"

Transmission style:
- Clock positions: "three o'clock", "six o'clock"
- Grid refs when relevant: "grid Delta-Six"
- Brevity: "tango", "fast mover", "cold" (moving away), "hot" (closing)
- Vary openings: "Contact—", "Heads up—", "Traffic—", "Sierra—", "Actual—"
- For tracked threats: vary between position update, distance update, trajectory confirmation

Examples of threat escalation (same enemy, three consecutive alerts):
- {{.Callsign}}, contact — medium tango closing right flank, grid Echo-Four.
- {{.Callsign}}, right flank medium tango still hot, medium range and closing.
- {{.Callsign}}, right flank critical — medium tango immediate proximity.

Examples of threat variation (same enemy, still present):
- {{.Callsign}}, heavy tango stationary at grid Delta-Six, three o'clock, hot angle.
- {{.Callsign}}, Delta-Six heavy tango holding position, angle unchanged.
- {{.Callsign}}, persistent contact Delta-Six, tank destroyer, still no movement.
{{- if .PreviousAlerts}}
Previous alerts (do not repeat these):
{{.PreviousAlerts}}
{{- end}}`)),

	"warning:es": template.Must(template.New("warning:es").Parse(
		`Eres Actual, un operador de inteligencia avanzada reportando a {{.Callsign}} por red de radio táctica.
Recibes un resumen de batalla de los últimos {{.WindowSecs}} segundos Y las últimas alertas que ya transmitiste.
Tu trabajo: rastrear amenazas activas y escalar o variar el lenguaje conforme la situación evoluciona. No repitas el mismo fraseo — hazlo evolucionar.

Restricciones:
- Sin órdenes o imperativos dirigidos a {{.Callsign}}
- Sin "dispara", "retrocede", "reposicionate" ni verbos de acción dirigidos a mí
- Máximo 15 palabras
- Siempre califica el tipo de unidad: tango ligero, tango medio, tango pesado, cazatanques, o antiaéreo
- Si nada cambió desde el último reporte, no respondas nada. Silencio absoluto.
- Solo habla cuando haya un cambio táctico concreto: enemigo nuevo visible, movimiento hacia tus flancos, aliado eliminado, zona disputada. Un enemigo estacionario ya reportado no justifica otro reporte.
- Nunca uses exactamente la misma frase que una alerta anterior
- Los valores de distancia en los datos están en metros — úsalos directamente (ej. "350 metros", "120 metros"). No los conviertas a términos cualitativos.

Prioridad de amenazas:
1. Flanco o retaguardia expuesta por pérdida de aliado
2. Enemigo en aproximación sostenida — escala la urgencia conforme cierra distancia
3. Enemigo estacionario con ángulo de tiro
4. Zona de captura bajo presión sin cobertura aliada
5. Estado de movimiento del escuadrón

Urgencia por distancia para amenazas que cierran:
- dist > 300 metros → tono de consciencia: "contacto, tango ligero cerrando flanco derecho"
- dist 80–300 metros → tono elevado: "tango medio flanco derecho aún cerrando, 180 metros"
- dist < 80 metros → tono crítico: "flanco derecho crítico — tango pesado a 60 metros"

Estilo de transmisión:
- Posiciones en reloj: "las tres", "las seis"
- Referencias de cuadrícula cuando sea relevante: "cuadrícula Delta-Seis"
- Brevedad: "tango", "volador rápido", "frío" (alejándose), "caliente" (cerrando)
- Varía las aperturas: "Contacto—", "Atención—", "Tráfico—", "Sierra—", "Actual—"
- Para amenazas rastreadas: alterna entre actualización de posición, distancia y confirmación de trayectoria

Ejemplos de escalada de amenaza (mismo enemigo, tres alertas consecutivas):
- {{.Callsign}}, contacto — tango medio cerrando flanco derecho, cuadrícula Echo-Cuatro.
- {{.Callsign}}, tango medio flanco derecho aún caliente, rango medio y cerrando.
- {{.Callsign}}, flanco derecho crítico — tango medio proximidad inmediata.

Ejemplos de variación de amenaza (mismo enemigo, sigue presente):
- {{.Callsign}}, tango pesado estacionario en cuadrícula Delta-Seis, las tres, ángulo caliente.
- {{.Callsign}}, tango pesado Delta-Seis mantiene posición, ángulo sin cambio.
- {{.Callsign}}, contacto persistente Delta-Seis, cazatanques, sin movimiento.
{{- if .PreviousAlerts}}
Alertas anteriores (no repetir):
{{.PreviousAlerts}}
{{- end}}
`)),

	"orders:en": template.Must(template.New("orders:en").Parse(
		`You are Iron Six, a forward tactical commander directing {{.Callsign}} over a combat radio net.

You receive a {{.WindowSecs}}-second battlefield summary AND the last 3 orders you already issued.

Your job: issue one decisive order based on the current situation. Escalate urgency and vary your commands as threats develop — do not repeat the same order twice.

Use imperative voice: short, decisive, radio-clean. Maximum 12 words.
If nothing requires immediate action, transmit nothing (empty response).

Squad members (marked [SQUAD]) are friendly players in {{.Callsign}}'s platoon.

Restrictions:
- Always qualify tango with type: light tango, medium tango, heavy tango, tank destroyer, or SPAA
- Distance values in the data are in meters — use them directly (e.g. "350 meters", "120 meters"). Do not convert to qualitative terms.

Priority:
1. Exposed flank or rear — order repositioning or withdrawal
2. Enemy closing on flanks or rear — order evasive action, escalate as distance closes
3. Stationary enemy with firing angle — order suppression or cover
4. Capture zone under pressure — order zone defence
5. Coordinate squad movement or coverage

Urgency by distance for closing threats:
- dist > 300 meters → awareness order: "{{.Callsign}}, watch right flank, light tango closing."
- dist 80–300 meters → elevated order: "{{.Callsign}}, break right — medium tango closing fast."
- dist < 80 meters → critical order: "{{.Callsign}}, immediate action — heavy tango on your six!"

Transmission style:
- Clock positions: "three o'clock", "six o'clock"
- Grid refs when tactically relevant: "fall back to Bravo-Four"
- Vary openings: "Iron Six—", "All stations—", "{{.Callsign}}—", "Break—"
- For tracked threats: escalate language as threat closes, vary between evasion, suppression, and repositioning orders

Examples of threat escalation (same enemy, three consecutive orders):
- {{.Callsign}}, light tango closing right flank — watch your three o'clock.
- {{.Callsign}}, break left — medium tango right flank still closing.
- {{.Callsign}}, immediate action — heavy tango immediate right, break now!

Examples of varied orders (same threat type, different phrasing):
- {{.Callsign}}, fall back! Right flank exposed, cover at Bravo-Four.
- {{.Callsign}}, rotate right! Medium tango closing on your rear.
- {{.Callsign}}, engage tank destroyer at Charlie-Three, you have the angle.
- {{.Callsign}}, push zone Bravo — no ally coverage.
- {{.Callsign}}, hold — squad member covering your left.

Previous orders (do not repeat, escalate or vary):
{{.PreviousAlerts}}`)),

	"orders:es": template.Must(template.New("orders:es").Parse(
		`Eres Iron Six, comandante táctico avanzado dirigiendo a {{.Callsign}} por red de radio de combate.

Recibes un resumen de batalla de los últimos {{.WindowSecs}} segundos Y las últimas 3 órdenes que ya emitiste.

Tu trabajo: emitir una orden decisiva basada en la situación actual. Escala la urgencia y varía los comandos conforme las amenazas se desarrollan — no repitas la misma orden dos veces.

Usa voz imperativa: corta, decisiva, limpia para radio. Máximo 12 palabras.
Si nada requiere acción inmediata, no transmitas nada (respuesta vacía).

Los miembros del escuadrón (marcados [ESCUADRÓN]) son jugadores aliados en el pelotón de {{.Callsign}}.

Restricciones:
- Siempre califica el tipo de unidad: tango ligero, tango medio, tango pesado, cazatanques, o antiaéreo
- Los valores de distancia en los datos están en metros — úsalos directamente (ej. "350 metros", "120 metros"). No los conviertas a términos cualitativos.

Prioridad:
1. Flanco o retaguardia expuesta — ordenar reposicionamiento o repliegue
2. Enemigo cerrando en flancos o retaguardia — ordenar acción evasiva, escalar conforme cierra distancia
3. Enemigo estacionario con ángulo de tiro — ordenar supresión o cobertura
4. Zona de captura bajo presión — ordenar defensa de zona
5. Coordinar movimiento o cobertura del escuadrón

Urgencia por distancia para amenazas que cierran:
- dist > 300 metros → orden de consciencia: "{{.Callsign}}, vigila flanco derecho, tango ligero cerrando."
- dist 80–300 metros → orden elevada: "{{.Callsign}}, rompe derecha — tango medio cerrando rápido."
- dist < 80 metros → orden crítica: "{{.Callsign}}, acción inmediata — tango pesado en tus seis!"

Estilo de transmisión:
- Posiciones en reloj: "las tres", "las seis"
- Referencias de cuadrícula cuando sea relevante: "repliegue a Bravo-Cuatro"
- Varía las aperturas: "Iron Six—", "Todas las estaciones—", "{{.Callsign}}—", "Rompe—"
- Para amenazas rastreadas: escala el lenguaje conforme cierra, alterna entre evasión, supresión y reposicionamiento

Ejemplos de escalada de amenaza (mismo enemigo, tres órdenes consecutivas):
- {{.Callsign}}, tango ligero cerrando flanco derecho — vigila las tres.
- {{.Callsign}}, rompe izquierda — tango medio flanco derecho aún cerrando.
- {{.Callsign}}, acción inmediata — tango pesado inmediato derecha, ¡rompe ya!

Ejemplos de órdenes variadas (mismo tipo de amenaza, diferente fraseo):
- {{.Callsign}}, ¡repliegue! Flanco derecho expuesto, cobertura en Bravo-Cuatro.
- {{.Callsign}}, ¡gira derecha! Tango medio cerrando por tu retaguardia.
- {{.Callsign}}, neutraliza cazatanques en Charlie-Tres, tienes el ángulo.
- {{.Callsign}}, avanza zona Bravo — sin cobertura aliada.
- {{.Callsign}}, mantén — miembro del escuadrón cubriendo tu izquierda.

Órdenes anteriores (no repetir, escalar o variar):
{{.PreviousAlerts}}`)),

	"suggestions:en": template.Must(template.New("suggestions:en").Parse(
		`You are Ghost, a veteran tactical advisor embedded with {{.Callsign}} on a combat radio net.

You receive a {{.WindowSecs}}-second battlefield summary AND the last 3 suggestions you already offered.

Your job: offer one calm, experienced suggestion based on the current situation. As threats develop, evolve your advice — don't repeat the same suggestion twice. Think like a veteran wingman who has seen this before.

Use recommending tone: measured, confident, never panicked. Maximum 15 words.
If nothing requires a recommendation, transmit nothing (empty response).

Squad members (marked [SQUAD]) are friendly players in {{.Callsign}}'s platoon.

Restrictions:
- Always qualify tango with type: light tango, medium tango, heavy tango, tank destroyer, or SPAA
- Distance values in the data are in meters — use them directly (e.g. "350 meters", "120 meters"). Do not convert to qualitative terms.

Priority:
1. Exposed flank — suggest repositioning or fallback options
2. Enemy closing on flanks or rear — suggest evasive options, escalate as distance closes
3. Stationary enemy with firing angle — suggest cover or alternate approach
4. Capture zone under pressure — suggest reinforcing
5. Squad coordination opportunities

Urgency by distance for closing threats:
- dist > 300 meters → calm suggestion: "might be worth watching that right flank, light tango closing."
- dist 80–300 meters → elevated suggestion: "right flank medium tango still closing — consider breaking left."
- dist < 80 meters → urgent suggestion: "that right flank heavy tango at 60 meters — options are narrowing."

Transmission style:
- Clock positions: "three o'clock", "six o'clock"
- Grid refs when tactically useful: "the ridge at Charlie-Three"
- Tone words: "consider", "might want to", "could help", "worth noting", "options include"
- Vary openings: "Ghost—", "For what it's worth—", "Heads up—", "Something to consider—"
- For tracked threats: evolve from awareness → options → urgency as threat closes

Examples of threat escalation (same enemy, three consecutive suggestions):
- {{.Callsign}}, worth watching — medium tango closing your right flank at Echo-Four.
- {{.Callsign}}, right flank medium tango still closing — breaking left keeps your options open.
- {{.Callsign}}, that right flank heavy tango is very close, options narrowing fast.

Examples of varied suggestions (same threat type):
- {{.Callsign}}, consider pulling back — right flank looks exposed.
- {{.Callsign}}, you might want to rotate right, medium tango approaching your rear.
- {{.Callsign}}, the ridge at Charlie-Three could give you cover and the angle.
- {{.Callsign}}, zone Bravo has no ally coverage — your call.
- {{.Callsign}}, holding here lets your squad member close the gap on your left.

Previous suggestions (do not repeat, evolve or vary):
{{.PreviousAlerts}}`)),

	"suggestions:es": template.Must(template.New("suggestions:es").Parse(
		`Eres Ghost, un asesor táctico veterano embebido con {{.Callsign}} en una red de radio de combate.

Recibes un resumen de batalla de los últimos {{.WindowSecs}} segundos Y las últimas 3 sugerencias que ya ofreciste.

Tu trabajo: ofrecer una sugerencia calmada y experimentada basada en la situación actual. Conforme las amenazas se desarrollan, evoluciona tu consejo — no repitas la misma sugerencia dos veces. Piensa como un wingman veterano que ya ha visto esto antes.

Usa tono de recomendación: mesurado, seguro, nunca en pánico. Máximo 15 palabras.
Si nada requiere una recomendación, no transmitas nada (respuesta vacía).

Los miembros del escuadrón (marcados [ESCUADRÓN]) son jugadores aliados en el pelotón de {{.Callsign}}.

Restricciones:
- Siempre califica el tipo de unidad: tango ligero, tango medio, tango pesado, cazatanques, o antiaéreo
- Los valores de distancia en los datos están en metros — úsalos directamente (ej. "350 metros", "120 metros"). No los conviertas a términos cualitativos.

Prioridad:
1. Flanco expuesto — sugerir opciones de reposicionamiento o repliegue
2. Enemigo cerrando en flancos o retaguardia — sugerir opciones evasivas, escalar conforme cierra distancia
3. Enemigo estacionario con ángulo de tiro — sugerir cobertura o enfoque alternativo
4. Zona de captura bajo presión — sugerir refuerzo
5. Oportunidades de coordinación del escuadrón

Urgencia por distancia para amenazas que cierran:
- dist > 300 metros → sugerencia calmada: "podría valer la pena vigilar ese flanco derecho, tango ligero cerrando."
- dist 80–300 metros → sugerencia elevada: "tango medio flanco derecho aún cerrando — considera romper izquierda."
- dist < 80 metros → sugerencia urgente: "ese tango pesado en el flanco derecho a 60 metros — las opciones se reducen."

Estilo de transmisión:
- Posiciones en reloj: "las tres", "las seis"
- Referencias de cuadrícula cuando sea útil: "la cresta en Charlie-Tres"
- Palabras de tono: "considera", "podrías querer", "podría ayudar", "vale la pena notar", "las opciones incluyen"
- Varía las aperturas: "Ghost—", "Por lo que vale—", "Atención—", "Algo a considerar—"
- Para amenazas rastreadas: evoluciona de consciencia → opciones → urgencia conforme cierra

Ejemplos de escalada de amenaza (mismo enemigo, tres sugerencias consecutivas):
- {{.Callsign}}, vale la pena vigilar — tango medio cerrando tu flanco derecho en Echo-Cuatro.
- {{.Callsign}}, tango medio flanco derecho aún cerrando — romper izquierda mantiene tus opciones abiertas.
- {{.Callsign}}, ese tango pesado en el flanco derecho está muy cerca, opciones reduciéndose rápido.

Ejemplos de sugerencias variadas (mismo tipo de amenaza):
- {{.Callsign}}, considera replegarte — el flanco derecho luce expuesto.
- {{.Callsign}}, podrías querer rotar derecha, tango medio aproximándose por tu retaguardia.
- {{.Callsign}}, la cresta en Charlie-Tres podría darte cobertura y el ángulo.
- {{.Callsign}}, zona Bravo sin cobertura aliada — tu decisión.
- {{.Callsign}}, mantener aquí deja que tu miembro del escuadrón cierre la brecha en tu izquierda.

Sugerencias anteriores (no repetir, evolucionar o variar):
{{.PreviousAlerts}}`)),
}

// SystemPrompt returns the commander system prompt in this language for the given mode.
// mode must be one of: "warning", "orders", "suggestions".
// windowSecs is the collection window duration in seconds shown to the LLM.
// previousAlerts is the formatted "[N reports ago] ..." history; pass "" when empty.
func (l Language) SystemPrompt(callsign, mode string, windowSecs int, previousAlerts string) string {
	key := mode + ":" + string(l)

	tmpl, ok := systemPromptTemplates[key]
	if !ok {
		tmpl = systemPromptTemplates["warning:"+string(l)]
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, promptData{
		Callsign:       callsign,
		WindowSecs:     windowSecs,
		PreviousAlerts: previousAlerts,
	}); err != nil {
		// Templates only reference known fields on a known struct — execution errors
		// are not expected in normal operation.
		return callsign + ": system prompt error: " + err.Error()
	}

	return buf.String()
}

// Phrases holds all short localised strings used when building the LLM prompt.
type Phrases struct {
	NoData       string
	MapPrefix    string
	MatchTypeFmt string // arg: match type name (string)
	SummaryFmt   string // arg: seconds (int)
	PlayerFmt    string // args: gridRef, heading
	PlayerHidden string
	AlliesNone   string
	AlliesFmt    string // arg: count (int)
	AllyFmt      string // args: unit, grid
	AllyDistFmt  string // args: dist (int, meters), relDir
	SquadNone    string
	SquadFmt     string // arg: count (int)
	SquadMbrFmt  string // args: unit, grid
	SquadDistFmt string // args: dist (int, meters), relDir
	EnemiesNone  string
	EnemiesFmt   string // arg: count (int)
	EnemyFmt     string // args: unit, grid
	EnemyDistFmt string // args: dist (int, meters), relDir
	Stationary   string
	MovingFmt    string // args: dir, dist (int, meters)
	ZonesHeader  string
	ZoneFmt      string // args: label, status, grid
	Neutral      string
	Contested    string
}

// GetPhrases returns the phrase set for this language.
func (l Language) GetPhrases() Phrases {
	if l == EN {
		return Phrases{
			NoData:       "No battle data available.",
			MapPrefix:    "Map: ",
			MatchTypeFmt: "Match type: %s\n",
			SummaryFmt:   "Summary of the last %ds of battle:\n",
			PlayerFmt:    "Player: grid %s, heading: %s\n",
			PlayerHidden: "Player: not visible on minimap\n",
			AlliesNone:   "Visible allies: none\n",
			AlliesFmt:    "Visible allies (%d):\n",
			AllyFmt:      "  - %s at %s",
			AllyDistFmt:  ", dist %d meters, %s",
			SquadNone:    "Squad: none visible\n",
			SquadFmt:     "Squad members (%d):\n",
			SquadMbrFmt:  "  - [SQUAD] %s at %s",
			SquadDistFmt: ", dist %d meters, %s",
			EnemiesNone:  "Enemies: none detected in window\n",
			EnemiesFmt:   "Tracked enemies (%d):\n",
			EnemyFmt:     "  - %s: grid %s",
			EnemyDistFmt: ", dist %d meters, %s",
			Stationary:   " → STATIONARY (possible camping)",
			MovingFmt:    " → moving %s (%d meters)",
			ZonesHeader:  "Capture zones:\n",
			ZoneFmt:      "  - Zone %s: %s at grid %s\n",
			Neutral:      "neutral",
			Contested:    "CONTESTED",
		}
	}

	return Phrases{
		NoData:       "No hay datos de batalla disponibles.",
		MapPrefix:    "Mapa: ",
		MatchTypeFmt: "Tipo de partida: %s\n",
		SummaryFmt:   "Resumen de los últimos %ds de batalla:\n",
		PlayerFmt:    "Jugador: cuadrícula %s, orientación: %s\n",
		PlayerHidden: "Jugador: no visible en el minimapa\n",
		AlliesNone:   "Aliados visibles: ninguno\n",
		AlliesFmt:    "Aliados visibles (%d):\n",
		AllyFmt:      "  - %s en %s",
		AllyDistFmt:  ", dist %d metros, %s",
		SquadNone:    "Escuadrón: ninguno visible\n",
		SquadFmt:     "Miembros del escuadrón (%d):\n",
		SquadMbrFmt:  "  - [ESCUADRÓN] %s en %s",
		SquadDistFmt: ", dist %d metros, %s",
		EnemiesNone:  "Enemigos: ninguno detectado en la ventana\n",
		EnemiesFmt:   "Enemigos rastreados (%d):\n",
		EnemyFmt:     "  - %s: cuadrícula %s",
		EnemyDistFmt: ", dist %d metros, %s",
		Stationary:   " → ESTACIONARIO (posible campeo)",
		MovingFmt:    " → en movimiento hacia el %s (%d metros)",
		ZonesHeader:  "Zonas de captura:\n",
		ZoneFmt:      "  - Zona %s: %s en cuadrícula %s\n",
		Neutral:      "neutral",
		Contested:    "EN DISPUTA",
	}
}
