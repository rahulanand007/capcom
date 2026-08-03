import { ErrorState } from "@/components/error-state"

export default function NotFound() {
  return (
    <ErrorState
      title="This page does not exist"
      message="The address may be outdated, or the item may have been removed."
    />
  )
}
