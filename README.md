# JsonSiteGo

A blazing-fast, super-simple static website/server generator 
powered by Go, Pico CSS, and a single, schema-validated JSON config.
---

#### 📣 🚀 Try **JsonSiteGo**: from zero to live static site in 1 minute!

---

## 🚀 Why Use JsonSiteGo?

- **Ultra-Simple:** Just describe your site in a JSON file—no HTML or frameworks required.
- **Beautiful:** All pages styled with [Pico CSS](https://picocss.com/) for instant modern look.
- **Configurable:** Everything – pages, menu, footer, social links, custom components – lives in your JSON.
- **Schema-Validated:** Mistakes are caught early, thanks to [JSON Schema](https://json-schema.org/) validation.

---

## ✨ Features

- ⚡ **Instant start:** One binary, zero dependencies.
- 🪄 **Config-driven:** Change structure/content by editing `config.json`.
- 🏷️ **Structured pages:** Menu, drafts, order, custom blocks/components (e.g., Accordion cards).
- 🧩 **Easy theming:** Built-in template/layout system.
- 🌗 **Theme toggle:** Light/dark mode via cookies.
- 🧪 **Strong validation:** Fails early if your config isn’t right – thanks to JSON Schema.
- 🚀 **Perfect for:** Docs, portfolios, landing pages, mini-sites, hackathons, demos, education.

---

## 🛠️ Quick Start

1. **Clone & build:**

    ```
    git clone https://github.com/lao-tseu-is-alive/go-simple-http-static-server.git
    cd go-simple-http-static-server
    go build -o jsonsitego main.go
    ```

2. **Edit `config.json`:**

    ```
    {
      "title": "My Awesome Site",
      "baseURL": "https://mysite.dev/",
      "pages": [
        {
          "route": "GET /",
          "title": "Home",
          "content": "Welcome to my Pico-powered site!",
          "template": "main_basic.gohtml",
          "layout": "base_layout",
          "showInMenu": true,
          "create_handler": true,
          "menuOrder": 10
        }
      ]
    }
    ```

3. **Run:**

    ```
    ./jsonsitego
    # Visit http://localhost:8888/
    ```

---

## 📁 Project Structure

- `main.go` — main server and logic.
- `config.json` — your site’s config.
- `config.schema.json` — defines/validates what’s allowed in config.
- `templates/` — Go html templates (layouts, pages, components).
- Sample components: Accordion cards and forms.

---

## 📝 Extending

- Add new templates in `templates/components/`.
- Define custom blocks in your JSON config under `custom_content`.
- PRs welcome for new content types and layouts!

---

## 🔐 License

GNU GENERAL PUBLIC LICENSE 3
Built with ❤️ by [@lao-tseu-is-alive](https://github.com/lao-tseu-is-alive).

---
