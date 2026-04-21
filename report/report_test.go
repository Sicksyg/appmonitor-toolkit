package report

import (
	"os"
	"path/filepath"
	"testing"

	"AppMonitor/models"
)

func TestMakeMarotoReport_WithMockData_WritesPDF(t *testing.T) {
	t.Helper()

	var appStoreDescription string = `I Min Læge-appen kan du nemt kontakte din praktiserende læge på både dine egne og dine børns vegne, og du får adgang til sundhedsdata registreret om dig og dine børn.
	Med Min Læge-appen får du adgang til følgende:

	- Oplysninger om og nem adgang til din praktiserende læge: Se adresse, kontaktoplysninger, åbningstider og eventuel ferieafløser. Du kan også ringe op til klinikken direkte fra appen
	- Oplysninger og nem adgang til lægevagten: Ring til lægevagten i din region (der er åben for akutte henvendelser uden for din praktiserende læges åbningstid)
	- Videokonsultationer: Afhold en videokonsultation med din praktiserende læge fra fx dit hjem
	- E-konsultationer: Stil spørgsmål og få svar fra din praktiserende læge ved at sende en e-konsultation
	- Tidsbestilling: Bestil tid eller aflys en booket aftale med din praktiserende læge – aftalen kan fx være en videokonsultation
	- Aftaler: Få overblik over dine kommende og tidligere aftaler i hele sundhedsvæsenet
	- Medicin: Se din aktuelle og tidligere medicin og anmod om fornyelse af recepter på medicin
	- Prøvesvar: Se resultater på udvalgte prøver
	- Vaccinationer: Se dine og dine børns vaccinationer
	- Henvisninger: Se aktuelle og tidligere henvisninger fra din praktiserende læge og find nemt den ønskede behandler, hvis du selv skal booke tid hos fx en privatpraktiserende speciallæge
	- Diagnoser og forløbsplaner: Se eventuelle diagnoser og forløbsplaner. Har du en forløbsplan for type 2-diabetes kan du få adgang til en funktion, hvor du tester og gætter eget blodsukker
	- Dine børns sundhedsdata: Se bl.a. dine børns e-konsultationer, aftaler, vaccinationer og medicin
	- Spørgeskemaer: Besvar spørgeskemaer. Er du fx gravid kan du besvare et spørgeskema forud for din første graviditetskonsultation hos din praktiserende læge
	- Ydelsesbeskeder: Læs mere om din kontakt med din praktiserende læge
	
	Vil du vide mere om udvalgte funktioner i Min Læge-appen? Læs med herunder.

	VIDEOKONSULTATIONER
	En af appens mange funktioner er videokonsultationer, som giver dig mulighed for at sidde fx derhjemme og tale med din egen læge over skærmen. For at få en videokonsultation skal du have en planlagt aftale med din læge om dette. Når du booker en tid, kan du spørge, om konsultationen kan foregå på video. Vær dog opmærksom på, at det altid vil være lægen, der vurderer, om den lægelige undersøgelse kræver dit fysiske fremmøde i klinikken.
	
	MEDICIN
	En anden af appens funktioner er muligheden for at se din og dine børns aktuelle og tidligere medicin og anmode om fornyelse af recepter på medicin.

	SE DINE PRØVESVAR
	I Min Læge-appen er det også muligt at se dine prøvesvar på et stort udvalg af prøver – fx blodprøver.`

	sdkMap := map[string][]string{
		"Firebase Analytics": {
			"FIRAnalytics",
			"FIRInstallations",
		},
		"Adjust": {
			"ADJActivityHandler",
			"ADJSessionParameters",
		},
	}

	permissionMap := map[string]models.PermissionDetail{
		"NSCameraUsageDescription": {
			CommonName:           "Camera",
			AppleDescription:     "Accesses camera hardware for image capture.",
			DeveloperDescription: "Used for profile photo and document scanning.",
			Category:             "Sensitive",
			PlistKey:             "NSCameraUsageDescription",
		},
		"NSLocationWhenInUseUsageDescription": {
			CommonName:           "Location (When In Use)",
			AppleDescription:     "Accesses user location while app is active.",
			DeveloperDescription: "Used to provide nearby content and localization.",
			Category:             "Tracking",
			PlistKey:             "NSLocationWhenInUseUsageDescription",
		},
	}

	outDir := filepath.Join("..", "tmp", "report_test_output")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("create output dir: %v", err)
	}

	outPath := filepath.Join(outDir, "mock_analysis_report.pdf")
	rm := NewManager(func(_, _ string) {})

	if err := rm.MakeMarotoReport("Mock App", "com.example.mockapp", appStoreDescription, outPath, sdkMap, permissionMap); err != nil {
		t.Fatalf("MakeMarotoReport failed: %v", err)
	}

	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read generated report: %v", err)
	}

	if len(content) == 0 {
		t.Fatal("generated report is empty")
	}

	if len(content) < 4 || string(content[:4]) != "%PDF" {
		t.Fatalf("generated file is not a PDF: %s", outPath)
	}

	t.Logf("Mock report generated: %s", outPath)
}
