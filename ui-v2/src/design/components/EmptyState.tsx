interface EmptyStateProps {
  title: string;
  detail: string;
}

export function EmptyState(props: EmptyStateProps) {
  return (
    <div class="empty-state" role="status">
      <h3>{props.title}</h3>
      <p>{props.detail}</p>
    </div>
  );
}
