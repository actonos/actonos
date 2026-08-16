export function BlobBackdrop() {
  return (
    <div className="absolute inset-0 overflow-hidden pointer-events-none -z-10" aria-hidden="true">
      <svg className="w-full h-full" viewBox="0 0 1000 700" fill="none" xmlns="http://www.w3.org/2000/svg">
        {/* Moss Green Blob */}
        <path
          d="M120 180C220 120 320 160 380 260C440 360 380 460 280 480C180 500 80 440 40 340C0 240 20 240 120 180Z"
          fill="#59e25d"
          className="opacity-40 mix-blend-multiply"
        />
        {/* Fuchsia Blob */}
        <path
          d="M680 140C780 80 880 140 920 240C960 340 900 420 800 460C700 500 620 420 600 320C580 220 580 200 680 140Z"
          fill="#e261e5"
          className="opacity-35 mix-blend-multiply"
        />
        {/* Hi-Yellow Accent Blob */}
        <path
          d="M420 280C500 240 580 260 620 340C660 420 600 480 520 500C440 520 380 460 360 380C340 300 340 320 420 280Z"
          fill="#ffe228"
          className="opacity-50 mix-blend-multiply"
        />
        {/* Deep Ink Blob */}
        <path
          d="M750 380C830 340 890 380 910 440C930 500 890 560 810 580C730 600 690 540 690 480C690 420 670 420 750 380Z"
          fill="#130e30"
          className="opacity-10 mix-blend-multiply"
        />
      </svg>
    </div>
  );
}
