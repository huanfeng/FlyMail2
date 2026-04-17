import { computed, reactive, watch } from 'vue';
import { palettes } from './themes';
import { getBoolean, getString, setBoolean, setString, KEYS } from '../../utils/storage';

// ...

const layoutConfig = reactive({
  preset: 'Aura',
  primary: 'emerald',
  surface: null as string | null,
  darkTheme: getBoolean(KEYS.UI_THEME_DARK, false),
  menuMode: 'static'
});

const layoutState = reactive({
  staticMenuDesktopInactive: false,
  overlayMenuActive: false,
  profileSidebarVisible: false,
  configSidebarVisible: false,
  staticMenuMobileActive: false,
  menuHoverActive: false,
  activeMenuItem: null as string | null
});

// ...

const applyTheme = (primary: string) => {
  const palette = palettes[primary];
  if (palette) {
    const root = document.documentElement;
    Object.keys(palette).forEach((key) => {
      const colorValue = palette[Number(key)];
      if (colorValue) {
        root.style.setProperty(`--p-primary-${key}`, colorValue);
      }
    });
    // Also set the main primary color variable which PrimeVue uses
    const mainColor = palette[500];
    if (mainColor) {
      root.style.setProperty('--p-primary-color', mainColor);
    }
  }
};

// ...



watch(() => layoutConfig.darkTheme, (newValue) => {
  setBoolean(KEYS.UI_THEME_DARK, newValue);
  if (newValue) {
    document.documentElement.classList.add('app-dark');
  } else {
    document.documentElement.classList.remove('app-dark');
  }
}, { immediate: true });



watch(() => layoutConfig.primary, (newValue) => {
  setString(KEYS.UI_THEME_PRIMARY, newValue);
  applyTheme(newValue);
});

watch(() => layoutConfig.surface, (newValue) => {
  if (newValue) setString(KEYS.UI_THEME_SURFACE, newValue);
});

// Load initial values
const initLayout = () => {
  const savedPrimary = getString(KEYS.UI_THEME_PRIMARY, '');
  const savedSurface = getString(KEYS.UI_THEME_SURFACE, '');

  if (savedPrimary) {
    layoutConfig.primary = savedPrimary;
    applyTheme(savedPrimary);
  } else {
    // Apply default theme
    applyTheme(layoutConfig.primary);
  }

  if (savedSurface) layoutConfig.surface = savedSurface;
};

initLayout();

export function useLayout() {
  const setPrimary = (value: string) => {
    layoutConfig.primary = value;
    applyTheme(value);
  };

  const setSurface = (value: string) => {
    layoutConfig.surface = value;
  };

  const setPreset = (value: string) => {
    layoutConfig.preset = value;
  };

  const setActiveMenuItem = (item: string | null) => {
    layoutState.activeMenuItem = item;
  };

  const onMenuToggle = () => {
    if (layoutConfig.menuMode === 'overlay') {
      layoutState.overlayMenuActive = !layoutState.overlayMenuActive;
    }

    if (window.innerWidth > 900) {
      layoutState.staticMenuDesktopInactive = !layoutState.staticMenuDesktopInactive;
    } else {
      layoutState.staticMenuMobileActive = !layoutState.staticMenuMobileActive;
    }
  };

  const isSidebarActive = computed(() => layoutState.overlayMenuActive || layoutState.staticMenuMobileActive);

  const isDarkTheme = computed(() => layoutConfig.darkTheme);

  return { layoutConfig, layoutState, onMenuToggle, isSidebarActive, isDarkTheme, setActiveMenuItem, setPrimary, setSurface, setPreset };
}
