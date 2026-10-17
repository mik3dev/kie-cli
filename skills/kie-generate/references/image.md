# Images with kie-cli

Always run `kie-cli models schema <model>` first: field names differ per model. Run with `kie-cli run <model> -p ... --out DIR` and read `files[].path`.

## Text to image

Prompt only:

`kie-cli run google/imagen4-fast -p prompt="a fox in snow, 35mm photo" -p aspect_ratio=16:9`

- Imagen 4 (`google/imagen4`, `-fast`, `-ultra`): `aspect_ratio` is required (`1:1`, `16:9`, `9:16`, `3:4`, `4:3`). Optional `negative_prompt`, `seed`.
- Presets: `@imagen-rapida` (draft), `@imagen-calidad` (final render); both default to `1:1`, pass `prompt` (and override `aspect_ratio` with `-p`).
- Other families exist (Seedream, Flux 2, Ideogram, GPT Image, Qwen, Grok). Find them with `kie-cli models search seedream` and read each schema: the size field is `aspect_ratio`, `image_size`, `resolution` or `quality` depending on the model, with different enums.
- Some models return several images (for example `num_images`, `max_images`, `n`). `files` then lists all of them.

## Edit / image-to-image

The input image field name differs by model; copy it from the schema:

- `image_urls` (array): `google/nano-banana-edit`, Seedream edit models, Qwen 3 edit. Repeat the key per image.
- `image_url` (string): Qwen image edit, Ideogram, Topaz upscale.
- `image_input` (array): `nano-banana-2`, `nano-banana-pro`. `input_urls` (array): Flux 2, GPT Image.
- Some models accept a single image under a plural name that is an array (the schema says so).

Local files are uploaded for you:

`kie-cli run google/nano-banana-edit -p prompt="make the sky sunset orange" -p image_urls=./photo.png -p output_format=png`

Preset: `kie-cli run @imagen-edicion -p prompt="..." -p image_urls=./photo.png` (needs `prompt` and `image_urls`).

Multi-image edit or style reference: repeat `-p image_urls=...` (see the model's maximum in `models schema`).

## Upscale and background removal

- `kie-cli run topaz/image-upscale -p image_url=./small.png -p upscale_factor=2` (`upscale_factor` is a string: `"1"`, `"2"` or `"4"`).
- `kie-cli run recraft/remove-background -p image=./portrait.png` (field is `image`; no other params).

## Tips

- Prompt length limits vary by model (hundreds to tens of thousands of chars). A long prompt can go through `-p prompt=@prompt.txt`.
- Generated files expire after about 14 days; download (default) and report local paths.
- `--dry-run` shows the price before spending.
