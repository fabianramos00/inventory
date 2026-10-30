package main

import (
	"context"
	"fmt"
	"os"

	"inventory-desktop/internal/application"
	"inventory-desktop/internal/bootstrap"
	"inventory-desktop/internal/config"
	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
)

const seedPageSize = 500

type categorySeed struct {
	Name        string
	Code        string
	Description string
}

type brandSeed struct {
	Name    string
	LogoURL *string
}

type unitSeed struct {
	Name         string
	Abbreviation string
}

var (
	categories = []categorySeed{
		{"Materiales de Construcción", "MATC", "Cemento, arena, ladrillo, bloque, concreto, mortero, grava, estuco."},
		{"Acabados y Pinturas", "ACAB", "Pintura, barniz, sellador, masilla, thinner, rodillos, brochas, cintas de enmascarar."},
		{"Electricidad", "ELEC", "Cable, interruptores, tomas, breakers, lámparas, tubería eléctrica, cajas de inspección."},
		{"Plomería y Gasfitería", "PLOM", "Tubería PVC/CPVC, codos, llaves de paso, sanitarios, grifería, flux, cinta teflón."},
		{"Herramientas Manuales", "HMAN", "Martillos, destornilladores, llaves fijas/ajustables, pinzas, niveles, metros, cinceles."},
		{"Herramientas Eléctricas", "HELC", "Taladros, pulidoras, sierras, soldadoras, lijadoras, baterías."},
		{"Ferretería General", "FERR", "Bisagras, cerraduras, chapas, manijas, ruedas, ángulos, perfiles metálicos, rejillas."},
		{"Tornillería y Anclajes", "TORN", "Tornillos, tuercas, clavos, pernos, anclajes, tarugos, grapas, pasadores."},
		{"Seguridad Industrial", "SEGI", "Casco, guantes, botas punta de acero, lentes, arnés, señalización, tapabocas."},
		{"Maderas y Tableros", "MADE", "Madera dimensionada, MDF, triplex, listones, vigas, melamina."},
		{"Techos y Cubiertas", "TECH", "Teja de zinc, teja plástica, policarbonato, membranas, impermeabilizantes, cumbreras."},
		{"Pisos y Enchapes", "PISO", "Cerámica, porcelanato, laminado, pegantes, boquillas, espátulas."},
		{"Jardinería", "JARD", "Abonos, semillas, mangueras, machetes, aspersores, macetas, insecticidas."},
		{"Aseo y Limpieza", "ASEO", "Detergentes, escobas, baldes, trapero, desinfectantes, bolsas."},
		{"Adhesivos y Selladores", "ADHE", "Pegantes, siliconas, selladores, epóxicos, pegante de PVC, pega loca."},
		{"Equipos y Maquinaria", "EQUIP", "Andamios, mezcladoras, compresores, escaleras, carretillas, vibradoras."},
		{"Cerrajería", "CERR", "Cerraduras, candados, cilindros, chapas de seguridad, cerrojos, llaves."},
		{"Cocina y Baño", "COCB", "Lavaplatos, sanitarios, accesorios de baño, grifería decorativa, sifones."},
	}

	brands = []brandSeed{
		{Name: "3M"},
		{Name: "Bosch"},
		{Name: "Black+Decker"},
		{Name: "Stanley"},
		{Name: "Truper"},
		{Name: "Urrea"},
		{Name: "Belmo"},
		{Name: "Tremix"},
		{Name: "Argos"},
		{Name: "Holcim"},
		{Name: "Pintuco"},
		{Name: "Comex"},
		{Name: "Imperlux"},
		{Name: "Genérico"},
		{Name: "Corona"},
		{Name: "Tecnoglass"},
		{Name: "Rex"},
		{Name: "Dexter"},
	}

	materials = []string{
		"Acero",
		"Acero inoxidable",
		"Acero al carbono",
		"Hierro",
		"Hierro galvanizado",
		"Aluminio",
		"Cobre",
		"Latón",
		"Bronce",
		"Plástico",
		"PVC",
		"Polipropileno (PP)",
		"Polietileno (PE)",
		"Madera",
		"Madera tratada",
		"Concreto",
		"Cemento",
		"Cerámica",
		"Porcelana",
		"Vidrio",
		"Goma",
		"Caucho",
		"Nylon",
		"Teflón (PTFE)",
		"Látex",
		"Acrílico",
		"Epóxico",
		"Poliuretano",
		"Yeso",
		"Fibra de vidrio",
		"Cuero",
		"Tela",
		"Papel",
		"Cartón",
		"Espuma",
		"Silicona",
		"Zinc",
		"Cromo",
		"Níquel",
		"Mixto / Compuesto",
	}

	measurementUnits = []unitSeed{
		{"Unidad", "und"},
		{"Docena", "doc"},
		{"Caja", "cja"},
		{"Paquete", "pqt"},
		{"Bolsa", "bl"},
		{"Saco", "sco"},
		{"Metro", "m"},
		{"Metro cuadrado", "m²"},
		{"Metro cúbico", "m³"},
		{"Centímetro", "cm"},
		{"Milímetro", "mm"},
		{"Pulgada", "plg"},
		{"Pie", "ft"},
		{"Yarda", "yd"},
		{"Kilogramo", "kg"},
		{"Gramo", "g"},
		{"Libra", "lb"},
		{"Quintal", "qq"},
		{"Tonelada", "t"},
		{"Litro", "L"},
		{"Mililitro", "mL"},
		{"Galón", "gal"},
		{"Caneca", "can"},
		{"Rollos", "rol"},
		{"Placa", "pla"},
		{"Lámina", "lam"},
		{"Barra", "bar"},
		{"Tubo", "tub"},
		{"Pares", "par"},
		{"Juego", "jgo"},
		{"Ciento", "cto"},
		{"Millar", "mill"},
		{"Kilovatio", "kW"},
		{"Ampere", "A"},
		{"Voltio", "V"},
		{"Hora", "h"},
		{"Porcentaje", "%"},
	}
)

type catalogPlan int

const (
	planCreate catalogPlan = iota
	planUpdate
	planSkip
)

// planCatalog matches a desired row against existing rows by name, then by the
// secondary key (category code or unit abbreviation).
func planCatalog[T any](
	items []*T,
	name string,
	alt string,
	nameOf func(*T) string,
	altOf func(*T) string,
	changed func(*T) bool,
) (*T, catalogPlan) {
	for _, item := range items {
		if nameOf(item) == name || (alt != "" && altOf(item) == alt) {
			if changed(item) {
				return item, planUpdate
			}
			return item, planSkip
		}
	}
	return nil, planCreate
}

type summary struct {
	label   string
	created int
	updated int
	total   int
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logger := cfg.Logger()

	ctx := context.Background()
	stores, err := bootstrap.Open(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("database startup failed: %w", err)
	}
	defer stores.Closer()

	steps := []struct {
		label string
		run   func() (summary, error)
	}{
		{"categories", func() (summary, error) {
			return seedCategories(ctx, application.NewCategoryService(stores.Categories))
		}},
		{"brands", func() (summary, error) {
			return seedBrands(ctx, application.NewBrandService(stores.Brands))
		}},
		{"materials", func() (summary, error) {
			return seedMaterials(ctx, application.NewMaterialService(stores.Materials))
		}},
		{"measurement units", func() (summary, error) {
			return seedMeasurementUnits(ctx, application.NewMeasurementUnitService(stores.MeasurementUnits))
		}},
	}

	for _, step := range steps {
		fmt.Printf("\n--- Seeding %s ---\n", step.label)
		result, err := step.run()
		if err != nil {
			return fmt.Errorf("seed %s: %w", step.label, err)
		}
		fmt.Printf("--- Done: %d created, %d updated, %d total %s ---\n", result.created, result.updated, result.total, result.label)
	}
	return nil
}

func seedCategories(ctx context.Context, svc inbound.CatalogUseCase[domain.Category, inbound.CreateCategoryInput, inbound.UpdateCategoryInput]) (summary, error) {
	result := summary{label: "categories", total: len(categories)}
	items, err := listAll(ctx, svc)
	if err != nil {
		return result, err
	}

	for _, seed := range categories {
		code, description := seed.Code, seed.Description
		existing, plan := planCatalog(items, seed.Name, seed.Code,
			func(c *domain.Category) string { return c.Name },
			func(c *domain.Category) string { return stringValue(c.Code) },
			func(c *domain.Category) bool {
				return !sameString(c.Code, seed.Code) || !sameString(c.Description, seed.Description)
			},
		)
		switch plan {
		case planCreate:
			created, err := svc.Create(ctx, inbound.CreateCategoryInput{Name: seed.Name, Code: &code, Description: &description})
			if err != nil {
				return result, err
			}
			items = append(items, created)
			result.created++
		case planUpdate:
			if _, err := svc.Update(ctx, existing.ID, inbound.UpdateCategoryInput{Code: &code, Description: &description}); err != nil {
				return result, err
			}
			result.updated++
		case planSkip:
			result.updated++
		}
	}
	return result, nil
}

func seedBrands(ctx context.Context, svc inbound.CatalogUseCase[domain.Brand, inbound.CreateBrandInput, inbound.UpdateBrandInput]) (summary, error) {
	result := summary{label: "brands", total: len(brands)}
	items, err := listAll(ctx, svc)
	if err != nil {
		return result, err
	}

	for _, seed := range brands {
		logoURL := ""
		if seed.LogoURL != nil {
			logoURL = *seed.LogoURL
		}
		existing, plan := planCatalog(items, seed.Name, "",
			func(b *domain.Brand) string { return b.Name },
			func(*domain.Brand) string { return "" },
			func(b *domain.Brand) bool { return !sameString(b.LogoURL, logoURL) },
		)
		switch plan {
		case planCreate:
			created, err := svc.Create(ctx, inbound.CreateBrandInput{Name: seed.Name, LogoURL: seed.LogoURL})
			if err != nil {
				return result, err
			}
			items = append(items, created)
			result.created++
		case planUpdate:
			// An empty pointer normalizes to nil in the service, clearing the logo.
			if _, err := svc.Update(ctx, existing.ID, inbound.UpdateBrandInput{LogoURL: &logoURL}); err != nil {
				return result, err
			}
			result.updated++
		case planSkip:
			result.updated++
		}
	}
	return result, nil
}

func seedMaterials(ctx context.Context, svc inbound.CatalogUseCase[domain.Material, inbound.CreateMaterialInput, inbound.UpdateMaterialInput]) (summary, error) {
	result := summary{label: "materials", total: len(materials)}
	items, err := listAll(ctx, svc)
	if err != nil {
		return result, err
	}

	for _, name := range materials {
		_, plan := planCatalog(items, name, "",
			func(m *domain.Material) string { return m.Name },
			func(*domain.Material) string { return "" },
			func(*domain.Material) bool { return false },
		)
		if plan != planCreate {
			result.updated++
			continue
		}
		created, err := svc.Create(ctx, inbound.CreateMaterialInput{Name: name})
		if err != nil {
			return result, err
		}
		items = append(items, created)
		result.created++
	}
	return result, nil
}

func seedMeasurementUnits(ctx context.Context, svc inbound.CatalogUseCase[domain.MeasurementUnit, inbound.CreateMeasurementUnitInput, inbound.UpdateMeasurementUnitInput]) (summary, error) {
	result := summary{label: "measurement units", total: len(measurementUnits)}
	items, err := listAll(ctx, svc)
	if err != nil {
		return result, err
	}

	for _, seed := range measurementUnits {
		abbreviation := seed.Abbreviation
		existing, plan := planCatalog(items, seed.Name, seed.Abbreviation,
			func(u *domain.MeasurementUnit) string { return u.Name },
			func(u *domain.MeasurementUnit) string { return u.Abbreviation },
			func(u *domain.MeasurementUnit) bool { return u.Abbreviation != seed.Abbreviation },
		)
		switch plan {
		case planCreate:
			created, err := svc.Create(ctx, inbound.CreateMeasurementUnitInput{Name: seed.Name, Abbreviation: seed.Abbreviation})
			if err != nil {
				return result, err
			}
			items = append(items, created)
			result.created++
		case planUpdate:
			if _, err := svc.Update(ctx, existing.ID, inbound.UpdateMeasurementUnitInput{Abbreviation: &abbreviation}); err != nil {
				return result, err
			}
			result.updated++
		case planSkip:
			result.updated++
		}
	}
	return result, nil
}

func listAll[T any, C any, U any](ctx context.Context, svc inbound.CatalogUseCase[T, C, U]) ([]*T, error) {
	var all []*T
	skip := 0
	for {
		page, err := svc.List(ctx, skip, seedPageSize, "")
		if err != nil {
			return nil, err
		}
		all = append(all, page.Items...)
		if !page.HasNext || len(page.Items) == 0 {
			return all, nil
		}
		skip += len(page.Items)
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func sameString(value *string, want string) bool {
	return stringValue(value) == want
}
