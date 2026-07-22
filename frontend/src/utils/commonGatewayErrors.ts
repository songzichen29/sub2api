type CommonGatewayErrorResolver = (key: string, fallback: string) => string

let resolver: CommonGatewayErrorResolver | undefined

export function configureCommonGatewayErrorResolver(
  nextResolver: CommonGatewayErrorResolver,
): void {
  resolver = nextResolver
}

export function resolveCommonGatewayError(key: string, fallback: string): string {
  return resolver?.(key, fallback) ?? fallback
}
