import type { SVGProps } from 'react';

/** Mission Control logo mark: an "M" inside an orbital ring. Uses currentColor
 *  so it inherits text color; size it with height/width classes. */
export function LogoMark(props: SVGProps<SVGSVGElement>) {
  return (
    <svg
      viewBox="0 0 64 64"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden="true"
      {...props}
    >
      <ellipse
        cx="32"
        cy="38"
        rx="21"
        ry="8"
        stroke="currentColor"
        strokeWidth="3.2"
        fill="none"
      />
      <path
        d="M22 44 V26 L32 37 L42 26 V44"
        stroke="currentColor"
        strokeWidth="3.6"
        strokeLinecap="round"
        strokeLinejoin="round"
        fill="none"
      />
      <path d="M26 45.5 V48" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
      <path d="M38 45.5 V48" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
    </svg>
  );
}
