# Rainbow Statusline: projekt

Status: zaakceptowany przez właściciela 2026-11-01.

## Po co

Deweloper korzystający z asystenta AI w terminalu chce w jednym pasku widzieć, co ma włączone w bieżącej sesji, ile dają mu pluginy oszczędzające tokeny (Ponytail, Caveman, RTK, claude-mem) i w jakim stanie są sesja i cache. Właściciel ma taki pasek dla Claude Code i OpenCode na swoim Macu i uważa, że spodoba się innym. Istniejące narzędzia (`ccstatusline`, `copilot-statusline`, `agystatusline`) obsługują po jednym asystencie; nie znaleziono pobieżnym wyszukiwaniem niczego, co łączy kilku asystentów z podsumowaniami pluginów.

## Punkt wyjścia

Działający kod do przeniesienia, lokalnie u właściciela:

- `~/.claude/statusline-command.sh`: pasek Claude Code (bash, jq, awk, sqlite3)
- `~/.claude/statusline-caveman-total.pl`: Caveman `t:` z transkryptów Claude Code
- `~/.config/opencode/plugins/statusline/tui.tsx`: pasek OpenCode (plugin TUI w Solid/OpenTUI, slot `app_bottom`)

Wygląd docelowy to trzy linie w obecnych kolorach:

```
user@host:katalog
[PONYTAIL] [CAVEMAN] (c: ~156.1k | t: ~185.8M) [RTK] (c: 1.1k · 28.5% | t: 38.3M · 55.0%) [CLAUDE-MEM] (c: 403.2k · 87% | t: 67.5M · 92%)
Opus 5.5 (medium) │ ctx ▰▰▰▱▱▱▱▱ 118.1k / 200.0k (59.0%) · Σout 84.1k │ session ~$8.67 ‖ ⚡ read 8.2k · write 0 · ⏱ 2:32 1h
```

`c:` to bieżąca sesja, `t:` to suma ze wszystkich sesji. Kolory i progi (kontekst zielony <50%, żółty <80%, czerwony; timer cache; kolor effortu) są w obecnych plikach i są źródłem prawdy.

## Cele pierwszej wersji

Pierwsza wersja to moment, w którym właściciel może upublicznić repo.

1. Działa na **macOS i Windows**.
2. Działa w **Claude Code, OpenCode i GitHub Copilot CLI**.
3. Pokazuje stan i podsumowania Ponytail, Caveman, RTK, claude-mem oraz model, effort, kontekst, Σout, koszt i cache.
4. **Łatwa instalacja** na obu systemach.
5. **Konfiguracja w TUI**: jeden ekran, lista elementów i podgląd paska na żywo (układ A).

Najważniejsze są punkty 4 i 5.

## Poza pierwszą wersją

- Linux jako obiecana platforma (kod ma nie stawiać przeszkód, ale nie obiecujemy go ani nie wydajemy).
- Codex: `tui.status_line` przyjmuje tylko listę wbudowanych pól, bez własnego polecenia i kolorów; zgłoszenie [openai/codex#17827](https://github.com/openai/codex/issues/17827) jest otwarte. Wraca, gdy OpenAI to doda.
- Antigravity: ma `statusLine` z bogatym JSON-em, więc to następny kandydat.
- Wyłączanie pluginu jednym poleceniem na czas sesji (właściciel: opcjonalne).
- Zewnętrzny system wtyczek. Integracje dopisuje się w kodzie.
- Motywy.

## Czego nie wolno (antycele)

**Pasek nie może pokazać liczby, w którą nie da się uwierzyć**, np. 1500%. To najgorsza wada według właściciela, nawet jeśli reszta działa.

- Procent spoza 0–100, ujemne tokeny, NaN i wartości nie do sparsowania dają `—`. Nie wolno obcinać po cichu do 100%.
- Wartość, która nie jest pomiarem, ma `~`. Wyjątek z decyzji właściciela: claude-mem `t:` zostaje bez `~`, jak dziś, a README opisuje je jako potencjał.
- README opisuje wzór i źródło każdego pola.

Pozostałe:

- Pasek nigdy się nie wywraca i nie wypisuje błędów na ekran asystenta. Przypomnienie: `undefined === undefined` wywróciło cały TUI OpenCode na ekranie startowym.
- Pasek nie spowalnia asystenta.
- Żadnej telemetrii ani wysyłania danych poza komputer.
- Instalator nie może zepsuć ustawień asystenta.

## Decyzje właściciela

- Nazwa: Rainbow Statusline. Repo prywatne `milczarekIT/rainbow-statusline` na koncie osobistym, później publiczne.
- Język: Go, jeden plik wykonywalny bez zależności, z małym pluginem dla OpenCode.
- TUI: układ A.
- Pola Caveman (`Σout × 0.65/0.35`) i claude-mem `t:` (wzór samego claude-mem, potencjał) zostają jak dziś, mimo że zawyżają. README opisuje je jako szacunki.

## Działanie

**Jedno polecenie, dwa tryby.** `rainbow-statusline` uruchomione w terminalu otwiera TUI. Uruchomione przez asystenta (dane na stdin) rysuje pasek.

**Adaptery asystentów** zamieniają dane wejściowe na wspólny model sesji:

| Asystent | Wejście | Wyjście | Zapis ustawień |
|---|---|---|---|
| Claude Code | JSON na stdin | linie z ANSI | `statusLine` w `~/.claude/settings.json` |
| Copilot CLI | JSON na stdin; `statusLine` z `command`, `padding`, `refreshInterval` | linie z ANSI | `~/.copilot/settings.json` |
| OpenCode | plugin zbiera dane sesji z API TUI i przekazuje je do pliku wykonywalnego | segmenty JSON (tekst + kolor), które plugin rysuje przez `<span>` | `tui.json` + plik pluginu |

OpenCode nie potrafi uruchomić polecenia paska, więc mały plugin zostaje, ale nie liczy nic sam. Na Windows Copilot uruchamia polecenie przez `cmd.exe`.

**Integracje** to po jednym pliku Go na plugin, ze wspólnym interfejsem: czy jest zainstalowany, jaki ma tryb, ile daje `c:` i `t:`. Nowa integracja nie wymaga zmian poza rejestracją. Brak pluginu ukrywa odznakę (Ponytail, Caveman) albo pokazuje `[X off]` (RTK, claude-mem), jak dziś.

**Źródła danych** są takie jak w obecnym kodzie:

- RTK `t:`: `rtk gain -f json`. RTK `c:`: tabela `commands` w `history.db`, dopasowana przez `hook_decisions` (Claude Code) albo przez polecenia bash sesji (OpenCode).
- claude-mem `t:`: wzór na tabeli `observations`. `c:`: wiersz `Nt work | Nt saved` z transkryptu (Claude Code) albo z `/api/context/inject` (OpenCode).
- Caveman: flagi trybu, Σout, `~/.cache/opencode-statusline/cave-sessions` w OpenCode, transkrypty w Claude Code.

Ścieżki muszą wynikać z systemu, nie z Maca. SQLite ma być czytany biblioteką wbudowaną w plik wykonywalny, bo Windows nie ma `sqlite3`.

**Szybkość.** Rysowanie korzysta z cache i nie czeka na wolne dane; odświeżanie (`rtk gain`, skan transkryptów) działa w tle, jak dziś.

**Instalacja.** macOS przez `brew`, Windows przez `scoop`, plus pliki w GitHub Releases. Do tego skrypty jednowierszowe dla osób pobierających z GitHuba: `curl … | sh` na macOS, `irm … | iex` na Windows. Ustawienia asystentów zmienia dopiero zatwierdzenie w przewodniku albo `s` w TUI, po kopii zapasowej. Jeśli asystent ma już inny `statusLine`, program pokazuje jaki i pyta przed zastąpieniem. `rainbow-statusline uninstall` usuwa tylko nasze wpisy i przywraca poprzedni `statusLine`, nie cofa późniejszych zmian użytkownika. Po samym `brew uninstall` lub `scoop uninstall` ustawienia zostają; README każe najpierw uruchomić `uninstall`.

**Przewodnik pierwszego uruchomienia** (na życzenie właściciela). Gdy nie ma jeszcze konfiguracji, `rainbow-statusline` prowadzi przez cztery kroki: asystenci (wykryci zaznaczeni), co pokazywać (wykryte pluginy zaznaczone, z podglądem), gęstość (pełny, zwarty, minimum), podsumowanie zmian w plikach i instalacja. Wyjście przed zatwierdzeniem niczego nie zmienia.

**TUI (układ A)** otwiera się przy każdym kolejnym uruchomieniu. Na górze podgląd paska na żywo. Niżej po lewej elementy paska (włączanie, wariant, kolejność), po prawej wykryte asystenty. Niewykryty plugin jest opisany „nie wykryto”. Klawisze: `spacja` włącz, `←→` wariant, `⇧↑↓` kolejność, `s` zapisz i zainstaluj, `q` wyjdź.

**Konfiguracja** trafia do jednego pliku w katalogu konfiguracji użytkownika, czytelnego także przy ręcznej edycji.

## Znane niewiadome do sprawdzenia przed budową

- Copilot: dokładne pola JSON na stdin i czy nadal potrzebna jest flaga eksperymentalna (`copilot help config` w 1.0.105 opisuje `statusLine`, wpis z maja 2026 wspomina o fladze `STATUS_LINE`).
- Copilot: gdzie są dane o pluginach i jak działają tam Ponytail, Caveman, RTK i claude-mem. Pola bez źródła pokazują `—`.
- Windows: położenie baz i flag RTK, claude-mem, Caveman i Ponytail oraz działanie pluginów OpenCode.
- Czy OpenCode pozwala pluginowi TUI uruchamiać plik wykonywalny na Windows bez migania okna.

## Co blokuje upublicznienie

Propozycja autora, właściciel nie zgłosił uwag:

- **Blokuje:** Claude Code nie działa na którymś systemie albo pasek nie działa w ogóle w Copilot CLI.
- **Nie blokuje:** wymagana flaga eksperymentalna w Copilocie (instalator ją ustawia), brak danych pluginów w Copilocie (`—`), problemy OpenCode na Windows (ta kombinacja dostaje status „jeszcze nie”). README podaje, co działa gdzie.

## Środowisko testowe

Właściciel zapewni dostęp do Windowsa i konta Copilot od 2026-11-02. Prace na Macu mogą ruszyć wcześniej; sprawdzenia na Windows i w Copilocie mają być opisane jako gotowe kroki dla właściciela, ze skryptem zbierającym próbki danych.

## Jak poznamy, że jest gotowe

1. Na Macu właściciela nowy pasek zastępuje oba obecne, a te same pola w tej samej sesji dają te same wartości. Jedyna celowa różnica to `—` zamiast wartości niemożliwej.
2. Na czystym macOS i Windows instalacja z menedżera pakietów i TUI kończą się działającym paskiem w trzech asystentach bez ręcznej edycji plików.
3. Testy na prawdziwych próbkach danych każdej integracji, także pustych i uszkodzonych: ani razu wartość niemożliwa, ani razu wywrotka.
4. Odinstalowanie przywraca ustawienia asystentów bajt w bajt, jeśli użytkownik nie zmienił ich po instalacji; w przeciwnym razie znikają tylko nasze wpisy, a jego zmiany zostają.

## Decyzje autora projektu (do sprawdzenia przez właściciela)

- Biblioteka TUI: Bubble Tea i pokrewne (ekosystem Go, działa na Windows).
- Odczyt SQLite: biblioteka w czystym Go, by nie wymagać kompilatora C.
- Interfejs TUI i README po angielsku, bo odbiorcy są globalni.
- Licencja MIT w chwili upublicznienia.
- Testy na GitHub Actions na macOS i Windows.
- Windows Terminal jako obsługiwany terminal; starsze konsole mogą pokazać gorsze znaki.
- Zachowanie instalacji, odinstalowania i istniejącego `statusLine` opisane wyżej.
- Próbki danych w testach są czyszczone z prywatnych treści przed commitem, bo repo będzie publiczne.

## Co zostawiamy budującemu

Układ pakietów, format pliku konfiguracji, nazwy wariantów pól, mechanizm odświeżania w tle i szczegóły wydawania.
