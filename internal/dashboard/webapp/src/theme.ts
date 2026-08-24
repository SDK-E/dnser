import { createTheme, MantineColorsTuple } from '@mantine/core'

const moss: MantineColorsTuple = [
  '#e3fbe0',
  '#d0f4cf',
  '#a4eba6',
  '#74e17c',
  '#4cd758',
  '#2cdb16',
  '#1fc70b',
  '#14af06',
  '#059c00',
  '#008a04',
]

const surface: MantineColorsTuple = [
  '#f2f7ef',
  '#e0ebdc',
  '#bdd4ba',
  '#96bc92',
  '#74a970',
  '#599554',
  '#458a40',
  '#33772f',
  '#266826',
  '#14590f',
]

export const theme = createTheme({
  primaryColor: 'moss',
  primaryShade: 5,
  colors: {
    moss,
    surface,
  },
  defaultRadius: 'md',
  fontFamily:
    '-apple-system, BlinkMacSystemFont, "SF Pro Text", "Segoe UI", Roboto, sans-serif',
  headings: {
    fontFamily:
      '-apple-system, BlinkMacSystemFont, "SF Pro Display", "Segoe UI", Roboto, sans-serif',
    fontWeight: '600',
  },
})
