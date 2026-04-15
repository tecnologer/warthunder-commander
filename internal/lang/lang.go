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
		return "Enemy (" + icon + ")"
	}

	return "Enemigo (" + icon + ")"
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
		return "Enemies (" + icon + ")"
	}

	return "Enemigos (" + icon + ")"
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

// promptData is the data passed to each system-prompt template.
type promptData struct {
	Callsign   string
	WindowSecs int
}

// systemPromptTemplates maps "mode:lang" to a compiled template.
// Templates are parsed once at init time; a malformed template panics early.
var systemPromptTemplates = map[string]*template.Template{ //nolint:gochecknoglobals // package-level lookup table, initialised once
	"warning:en": template.Must(template.New("warning:en").Parse(
		`You are my tactical intelligence operator in War Thunder. You refer to me as {{.Callsign}}.
You receive a tactical summary of the last {{.WindowSecs}} seconds. Issue the most critical situational warning: describe what is happening, not what I should do.
No orders, no imperatives, no "shoot now!", "get out!", "reposition", or action verbs directed at me.
Tactical information only. Maximum 15 words. Like over radio. If there are no changes between reports, respond with an empty message.

Squad members (marked [SQUAD]) are friendly players in my platoon. Note when a squad member is moving in the same direction as me or covering my flank.

Priority:
1. Flank or exposed rear due to ally eliminated or retreated
2. Enemy in sustained movement toward my flanks or rear
3. STATIONARY enemy with firing angle toward me
4. Capture zone under pressure without ally coverage
5. Squad member nearby and advancing with me, or squad member falling behind

Examples:
- {{.Callsign}}, right flank uncovered, ally that was covering has been eliminated.
- {{.Callsign}}, light tank on sustained trajectory toward your rear.
- {{.Callsign}}, stationary tank destroyer with line of fire to your right.
- {{.Callsign}}, left flank threat neutralized, route clear.
- {{.Callsign}}, enemy medium tank immediately ahead, minimal distance.
- {{.Callsign}}, squad member advancing with you to the north.
- {{.Callsign}}, squad member stationary, you are advancing alone.
`)),

	"warning:es": template.Must(template.New("warning:es").Parse(
		`Eres mi operador de inteligencia táctica en War Thunder. Te refieres a mí como {{.Callsign}}.
Recibes un resumen táctico de los últimos {{.WindowSecs}} segundos. Emite la advertencia situacional más crítica: describe qué está pasando, no qué debo hacer.
Sin órdenes, sin imperativos, sin "¡dispara ya!", "¡sal ya!", "reposiciónate", ni verbos de acción dirigidos a mí.
Solo información táctica. Máximo 15 palabras. Como por radio. Si no hay cambios, entre un reporte y otro, responder con un mensaje vacío.

Los miembros del escuadrón (marcados [ESCUADRÓN]) son jugadores aliados en mi pelotón. Notifica cuando un compañero de escuadrón se mueve en la misma dirección que yo o cubre mi flanco.

Prioridad:
1. Flanco o retaguardia expuesta por aliado eliminado o retirado
2. Enemigo en movimiento sostenido hacia mis flancos o retaguardia
3. Enemigo ESTACIONARIO en ángulo de tiro hacia mí
4. Zona de captura bajo presión sin cobertura aliada
5. Compañero de escuadrón avanzando conmigo o quedándose atrás

Ejemplos:
- {{.Callsign}}, flanco derecho sin cobertura, aliado que cubría fue eliminado.
- {{.Callsign}}, tanque ligero con trayectoria sostenida hacia tu retaguardia.
- {{.Callsign}}, cazatanques estacionario con línea de tiro por tu derecha.
- {{.Callsign}}, amenaza flanco izquierdo neutralizada, ruta despejada.
- {{.Callsign}}, tanque medio enemigo a tu frente inmediato, distancia mínima.
- {{.Callsign}}, compañero de escuadrón avanzando contigo hacia el norte.
- {{.Callsign}}, compañero de escuadrón estacionario, avanzas solo.
`)),

	"orders:en": template.Must(template.New("orders:en").Parse(
		`You are my tactical commander in War Thunder. You refer to me as {{.Callsign}}.
You receive a tactical summary of the last {{.WindowSecs}} seconds. Issue the single most critical direct order based on the battlefield situation.
Use imperative voice: short, decisive commands. Maximum 12 words. Like over radio.
If nothing requires immediate action, respond with an empty message.

Squad members (marked [SQUAD]) are friendly players in my platoon.

Priority:
1. Expose a covered flank — order repositioning or withdrawal
2. Enemy closing on flanks or rear — order evasive action
3. Stationary enemy with firing angle — order suppression or cover
4. Capture zone under pressure — order zone defence
5. Coordinate squad movement or coverage

Examples:
- {{.Callsign}}, fall back! Right flank exposed, take cover at B4.
- {{.Callsign}}, rotate right! Enemy tank closing on your rear.
- {{.Callsign}}, engage the tank destroyer at C3, you have the angle.
- {{.Callsign}}, push to zone B, no ally coverage there.
- {{.Callsign}}, hold position, squad member covering your left.
`)),

	"orders:es": template.Must(template.New("orders:es").Parse(
		`Eres mi comandante táctico en War Thunder. Te refieres a mí como {{.Callsign}}.
Recibes un resumen táctico de los últimos {{.WindowSecs}} segundos. Emite la orden directa más crítica basada en la situación del campo de batalla.
Usa voz imperativa: órdenes cortas y decisivas. Máximo 12 palabras. Como por radio.
Si nada requiere acción inmediata, responder con un mensaje vacío.

Los miembros del escuadrón (marcados [ESCUADRÓN]) son jugadores aliados en mi pelotón.

Prioridad:
1. Flanco expuesto — ordena reposicionamiento o retirada
2. Enemigo cerrando flancos o retaguardia — ordena acción evasiva
3. Enemigo estacionario con ángulo de tiro — ordena supresión o cobertura
4. Zona de captura bajo presión — ordena defensa de zona
5. Coordinar movimiento o cobertura del escuadrón

Ejemplos:
- {{.Callsign}}, ¡retrocede! Flanco derecho expuesto, cúbrete en B4.
- {{.Callsign}}, ¡gira a la derecha! Tanque enemigo cerrando por tu retaguardia.
- {{.Callsign}}, ataca al cazatanques en C3, tienes el ángulo.
- {{.Callsign}}, avanza a la zona B, no hay cobertura aliada.
- {{.Callsign}}, mantén posición, compañero de escuadrón cubre tu izquierda.
`)),

	"suggestions:en": template.Must(template.New("suggestions:en").Parse(
		`You are my tactical advisor in War Thunder. You refer to me as {{.Callsign}}.
You receive a tactical summary of the last {{.WindowSecs}} seconds. Offer the single most useful tactical suggestion based on the situation.
Use a recommending tone: "consider", "you might want to", "it could help to". Maximum 15 words. Like over radio.
If nothing requires a recommendation, respond with an empty message.

Squad members (marked [SQUAD]) are friendly players in my platoon.

Priority:
1. Exposed flank — suggest repositioning or fallback
2. Enemy closing on flanks or rear — suggest evasive options
3. Stationary enemy with firing angle — suggest cover or alternate route
4. Capture zone under pressure — suggest reinforcing
5. Squad coordination opportunities

Examples:
- {{.Callsign}}, consider pulling back — right flank looks exposed.
- {{.Callsign}}, you might want to rotate right, enemy tank approaching your rear.
- {{.Callsign}}, using the ridge at C3 for cover could give you the angle.
- {{.Callsign}}, zone B might benefit from your presence, no ally there.
- {{.Callsign}}, holding here lets your squad member close the gap.
`)),

	"suggestions:es": template.Must(template.New("suggestions:es").Parse(
		`Eres mi asesor táctico en War Thunder. Te refieres a mí como {{.Callsign}}.
Recibes un resumen táctico de los últimos {{.WindowSecs}} segundos. Ofrece la sugerencia táctica más útil basada en la situación.
Usa un tono de recomendación: "considera", "podrías", "convendría". Máximo 15 palabras. Como por radio.
Si nada requiere una recomendación, responder con un mensaje vacío.

Los miembros del escuadrón (marcados [ESCUADRÓN]) son jugadores aliados en mi pelotón.

Prioridad:
1. Flanco expuesto — sugiere reposicionamiento o retirada
2. Enemigo cerrando flancos o retaguardia — sugiere opciones evasivas
3. Enemigo estacionario con ángulo de tiro — sugiere cobertura o ruta alternativa
4. Zona de captura bajo presión — sugiere refuerzo
5. Oportunidades de coordinación de escuadrón

Ejemplos:
- {{.Callsign}}, considera retroceder, el flanco derecho parece expuesto.
- {{.Callsign}}, podrías girar a la derecha, tanque enemigo acercándose por la retaguardia.
- {{.Callsign}}, usar la cresta en C3 como cobertura podría darte el ángulo.
- {{.Callsign}}, la zona B podría beneficiarse de tu presencia, no hay aliados.
- {{.Callsign}}, mantener aquí le da tiempo a tu compañero de escuadrón a cerrar distancia.
`)),
}

// SystemPrompt returns the commander system prompt in this language for the given mode.
// mode must be one of: "warning", "orders", "suggestions".
// windowSecs is the collection window duration in seconds shown to the LLM.
func (l Language) SystemPrompt(callsign, mode string, windowSecs int) string {
	key := mode + ":" + string(l)

	tmpl, ok := systemPromptTemplates[key]
	if !ok {
		tmpl = systemPromptTemplates["warning:"+string(l)]
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, promptData{Callsign: callsign, WindowSecs: windowSecs}); err != nil {
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
	SummaryFmt   string // arg: seconds (int)
	PlayerFmt    string // args: gridRef, heading
	PlayerHidden string
	AlliesNone   string
	AlliesFmt    string // arg: count (int)
	AllyFmt      string // args: unit, grid
	AllyDistFmt  string // args: dist (float), relDir
	SquadNone    string
	SquadFmt     string // arg: count (int)
	SquadMbrFmt  string // args: unit, grid
	SquadDistFmt string // args: dist (float), relDir
	EnemiesNone  string
	EnemiesFmt   string // arg: count (int)
	EnemyFmt     string // args: unit, grid
	EnemyDistFmt string // args: dist (float), relDir
	Stationary   string
	MovingFmt    string // args: dir, dist (float)
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
			SummaryFmt:   "Summary of the last %ds of battle:\n",
			PlayerFmt:    "Player: grid %s, heading: %s\n",
			PlayerHidden: "Player: not visible on minimap\n",
			AlliesNone:   "Visible allies: none\n",
			AlliesFmt:    "Visible allies (%d):\n",
			AllyFmt:      "  - %s at %s",
			AllyDistFmt:  ", dist %.2f, %s",
			SquadNone:    "Squad: none visible\n",
			SquadFmt:     "Squad members (%d):\n",
			SquadMbrFmt:  "  - [SQUAD] %s at %s",
			SquadDistFmt: ", dist %.2f, %s",
			EnemiesNone:  "Enemies: none detected in window\n",
			EnemiesFmt:   "Tracked enemies (%d):\n",
			EnemyFmt:     "  - %s: grid %s",
			EnemyDistFmt: ", dist %.2f, %s",
			Stationary:   " → STATIONARY (possible camping)",
			MovingFmt:    " → moving %s (%.3f u)",
			ZonesHeader:  "Capture zones:\n",
			ZoneFmt:      "  - Zone %s: %s at grid %s\n",
			Neutral:      "neutral",
			Contested:    "CONTESTED",
		}
	}

	return Phrases{
		NoData:       "No hay datos de batalla disponibles.",
		MapPrefix:    "Mapa: ",
		SummaryFmt:   "Resumen de los últimos %ds de batalla:\n",
		PlayerFmt:    "Jugador: cuadrícula %s, orientación: %s\n",
		PlayerHidden: "Jugador: no visible en el minimapa\n",
		AlliesNone:   "Aliados visibles: ninguno\n",
		AlliesFmt:    "Aliados visibles (%d):\n",
		AllyFmt:      "  - %s en %s",
		AllyDistFmt:  ", dist %.2f, %s",
		SquadNone:    "Escuadrón: ninguno visible\n",
		SquadFmt:     "Miembros del escuadrón (%d):\n",
		SquadMbrFmt:  "  - [ESCUADRÓN] %s en %s",
		SquadDistFmt: ", dist %.2f, %s",
		EnemiesNone:  "Enemigos: ninguno detectado en la ventana\n",
		EnemiesFmt:   "Enemigos rastreados (%d):\n",
		EnemyFmt:     "  - %s: cuadrícula %s",
		EnemyDistFmt: ", dist %.2f, %s",
		Stationary:   " → ESTACIONARIO (posible campeo)",
		MovingFmt:    " → en movimiento hacia el %s (%.3f u)",
		ZonesHeader:  "Zonas de captura:\n",
		ZoneFmt:      "  - Zona %s: %s en cuadrícula %s\n",
		Neutral:      "neutral",
		Contested:    "EN DISPUTA",
	}
}
