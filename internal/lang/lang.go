// Package lang provides localised strings for the two supported UI languages.
package lang

import (
	"math"
	"strconv"
	"strings"
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

	bearing := math.Mod(math.Atan2(deltaY, deltaX)*180/math.Pi+90+360, 360)

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

// SystemPrompt returns the commander system prompt in this language.
func (l Language) SystemPrompt(callsign string) string {
	if l == EN {
		return `You are my tactical intelligence operator in War Thunder. You refer to me as ` + callsign + `.
You receive a tactical summary of the last 30 seconds. Issue the most critical situational warning: describe what is happening, not what I should do.
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
- ` + callsign + `, right flank uncovered, ally that was covering has been eliminated.
- ` + callsign + `, light tank on sustained trajectory toward your rear.
- ` + callsign + `, stationary tank destroyer with line of fire to your right.
- ` + callsign + `, left flank threat neutralized, route clear.
- ` + callsign + `, enemy medium tank immediately ahead, minimal distance.
- ` + callsign + `, squad member advancing with you to the north.
- ` + callsign + `, squad member stationary, you are advancing alone.
`
	}

	return `Eres mi operador de inteligencia táctica en War Thunder. Te refieres a mí como ` + callsign + `.
Recibes un resumen táctico de los últimos 30 segundos. Emite la advertencia situacional más crítica: describe qué está pasando, no qué debo hacer.
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
- ` + callsign + `, flanco derecho sin cobertura, aliado que cubría fue eliminado.
- ` + callsign + `, tanque ligero con trayectoria sostenida hacia tu retaguardia.
- ` + callsign + `, cazatanques estacionario con línea de tiro por tu derecha.
- ` + callsign + `, amenaza flanco izquierdo neutralizada, ruta despejada.
- ` + callsign + `, tanque medio enemigo a tu frente inmediato, distancia mínima.
- ` + callsign + `, compañero de escuadrón avanzando contigo hacia el norte.
- ` + callsign + `, compañero de escuadrón estacionario, avanzas solo.
`
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
