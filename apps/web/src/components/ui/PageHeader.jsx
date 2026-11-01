export default function PageHeader({ title, description, icon: Icon, children }) {
  return (
    <header className="mb-6 flex flex-wrap items-center justify-between gap-4">
      <div className="min-w-0">
        <div className="flex items-center gap-3">
          {Icon && <Icon aria-hidden="true" className="h-6 w-6 shrink-0 ui-text-secondary" />}
          <h1 className="text-2xl font-bold ui-text-primary">{title}</h1>
        </div>
        {description && <p className="mt-1 text-sm ui-text-muted">{description}</p>}
      </div>
      {children}
    </header>
  );
}
