// Shared by the root layout (server) and lib/appearance.ts (client), so this
// module must stay free of "use client".

export const THEME_KEY = "theme";
export const STYLE_KEY = "ui-style";
export const BORDERS_KEY = "borders";

/**
 * APPEARANCE_SCRIPT runs in <head> before first paint: it sets the .dark
 * class, data-style and data-borders on <html> from localStorage so there is
 * no flash of the wrong look. Keep it in step with apply() in
 * lib/appearance.ts. Defaults: system theme, Classic style, borders on.
 */
export const APPEARANCE_SCRIPT = `(function(){var r=document.documentElement;try{var t=localStorage.getItem('${THEME_KEY}');var d=t==='dark'||(t!=='light'&&window.matchMedia('(prefers-color-scheme: dark)').matches);r.classList.toggle('dark',d);if(localStorage.getItem('${STYLE_KEY}')==='neutral')r.setAttribute('data-style','neutral');if(localStorage.getItem('${BORDERS_KEY}')==='none')r.setAttribute('data-borders','none');}catch(e){}})();`;
