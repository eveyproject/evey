# Evey - Brand Kit

Système d'identité visuelle pour le projet Evey (self-hosted media server).

## Contenu

```
evey-brand/
├── svg/                          # Vectoriel - source de vérité
│   ├── evey-wordmark.svg                    # Logo principal (V rouge sur fond clair)
│   ├── evey-wordmark-mono.svg               # Sans rouge
│   ├── evey-wordmark-inverted.svg           # Sur fond noir
│   ├── evey-wordmark-inverted-mono.svg      # Sur fond noir sans rouge
│   ├── evey-wordmark-transparent.svg        # Fond transparent (mask)
│   └── evey-wordmark-transparent-white.svg  # Transparent, lettres blanches
├── png/                          # Wordmark en PNG (400 → 4000px)
├── favicon/
│   ├── favicon.svg               # SVG (recommandé navigateurs modernes)
│   ├── favicon.ico               # Multi-tailles 16/32/48/64/128
│   ├── favicon-{16..512}.png     # Toutes tailles standard
│   ├── apple-touch-icon-180.png  # iOS
│   └── android-chrome-{192,512}.png
├── play/                         # Google Play
│   ├── play-icon-512.png                    # Icône de l'appli (512×512)
│   └── play-feature-graphic-1024x500.png    # Image de présentation (1024×500)
└── social/
    ├── github-social.png         # 1280×640 (GitHub repo card)
    ├── og-image.png              # 1200×630 (Open Graph / og:image)
    ├── twitter-header.png        # 1500×500
    └── readme-hero.png           # 1600×400
```

## Couleurs

| Token       | Hex       | Usage                                    |
|-------------|-----------|------------------------------------------|
| `--evey-bg` | `#FFFFFF` | Fond clair                               |
| `--evey-fg` | `#0A0A0A` | Lettres E, Y                             |
| `--evey-v`  | `#DC2626` | Lettre V (référence Vendetta)            |
| `--evey-bg-dark` | `#0A0A0A` | Fond mode sombre                    |

## Typo

**Archivo Black** (Google Fonts, OFL) - alternative libre à Arial Black.
Les SVG embarquent les glyphes en `<path>` : aucune dépendance font côté client.

## Édition

Pas de `.psd` (format Adobe propriétaire, non reproductible librement).

Pour éditer le système :

1. **Inkscape** (gratuit, libre) - ouvre directement les `.svg`
2. **Figma** - File > Import > sélectionne le SVG
3. **Affinity Designer** - ouverture native SVG

Le SVG est la source. Pour régénérer les PNG après modif :
```bash
cairosvg evey-wordmark.svg -o output.png -W 2400
```

## Intégration web

### HTML head (favicon complet)
```html
<link rel="icon" type="image/svg+xml" href="/favicon/favicon.svg">
<link rel="icon" type="image/png" sizes="32x32" href="/favicon/favicon-32.png">
<link rel="icon" type="image/png" sizes="16x16" href="/favicon/favicon-16.png">
<link rel="apple-touch-icon" sizes="180x180" href="/favicon/apple-touch-icon-180.png">
<link rel="manifest" href="/favicon/site.webmanifest">
<meta property="og:image" content="/social/og-image.png">
<meta name="twitter:card" content="summary_large_image">
<meta name="twitter:image" content="/social/og-image.png">
```

### README GitHub
```markdown
<p align="center">
  <img src="docs/social/readme-hero.png" alt="Evey" width="800">
</p>
```

### GitHub repo social preview
Settings → Social preview → upload `social/github-social.png`.

## Règles d'usage

- Espace de respiration min. autour du logo = hauteur d'une lettre.
- La barre horizontale est constitutive du logo - ne pas la supprimer.
- Le V rouge est constitutif - ne pas le mettre en noir sauf dans la variante mono explicite.
- Ne pas étirer, déformer, ajouter d'ombre, de dégradé ou de glow.
- Ne pas modifier l'espacement entre les lettres.

## Licence

À définir par l'auteur du projet (MIT recommandé pour cohérence avec Evey).
