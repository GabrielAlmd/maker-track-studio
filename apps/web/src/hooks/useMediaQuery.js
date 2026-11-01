import { useEffect, useState } from 'react';

export default function useMediaQuery(query, fallback = true) {
  const getMatch = () => typeof window.matchMedia !== 'function' ? fallback : window.matchMedia(query).matches;
  const [matches, setMatches] = useState(getMatch);

  useEffect(() => {
    if (typeof window.matchMedia !== 'function') return undefined;
    const mediaQuery = window.matchMedia(query);
    const update = (event) => setMatches(event.matches);
    if (mediaQuery.addEventListener) {
      mediaQuery.addEventListener('change', update);
    } else {
      mediaQuery.addListener(update);
    }
    return () => {
      if (mediaQuery.removeEventListener) {
        mediaQuery.removeEventListener('change', update);
      } else {
        mediaQuery.removeListener(update);
      }
    };
  }, [query]);

  return matches;
}
