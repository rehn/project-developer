// App mark: a shell prompt on a dirt-yellow tile. Same artwork as app/icon.svg.
export default function Logo({ size = 24 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 64 64" aria-hidden="true" className="logo">
      <rect x="2" y="2" width="60" height="60" rx="14" fill="#B39130" />
      <path d="M17 22 L29 32 L17 42" fill="none" stroke="#2A2516" strokeWidth="6" strokeLinecap="round" strokeLinejoin="round" />
      <rect x="33" y="39" width="15" height="6" rx="3" fill="#2A2516" />
    </svg>
  );
}
