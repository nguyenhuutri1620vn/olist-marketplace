import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { fakeGateway } from '@/test/gateway'
import { renderApp } from '@/test/render'

let gw: ReturnType<typeof fakeGateway>

beforeEach(() => {
  sessionStorage.clear()
  gw = fakeGateway()
  vi.stubGlobal('fetch', vi.fn(gw.fetch))
})

afterEach(() => vi.unstubAllGlobals())

const table = () => screen.getByRole('table')
const bodyRows = () => within(table()).getAllByRole('row').slice(1)
const productCalls = () => gw.calls.map((c) => c.url).filter((u) => u.startsWith('/api/products'))

async function loaded() {
  await waitFor(() => expect(screen.queryAllByTestId('skeleton-cell')).toHaveLength(0))
}

describe('Products page', () => {
  it('shows 5 skeleton rows while the first page loads', async () => {
    renderApp('/')
    expect(screen.getByRole('heading', { name: 'Products' })).toBeInTheDocument()
    expect(within(table()).getAllByRole('row').slice(1)).toHaveLength(5)
    expect(screen.getAllByTestId('skeleton-cell').length).toBeGreaterThan(0)
    await loaded()
  })

  it('lists 20 Products per page with name, Category, Seller and price', async () => {
    renderApp('/')
    await loaded()
    const headers = within(table()).getAllByRole('columnheader').map((h) => h.textContent)
    expect(headers).toEqual(['Select', 'Name', 'Category', 'Seller', 'Price'])
    expect(bodyRows()).toHaveLength(20)
    const cells = within(bodyRows()[0]).getAllByRole('cell').map((c) => c.textContent)
    expect(cells).toEqual(['', 'health_beauty #0000', 'health_beauty', '6560211a · ibitinga, SP', 'R$ 1.000,00'])
    expect(screen.getByText('Page 1 of 3')).toBeInTheDocument()
    expect(productCalls()).toEqual(['/api/products?page=1&page_size=20'])
  })

  it('shows "Uncategorized" for Products without a dataset Category', async () => {
    renderApp('/?page=3')
    await loaded()
    const categories = bodyRows().map((r) => within(r).getAllByRole('cell')[2].textContent)
    expect(categories).toEqual(['health_beauty', 'health_beauty', 'health_beauty', 'health_beauty', 'health_beauty', 'Uncategorized', 'Uncategorized', 'Uncategorized'])
  })

  it('pages with Previous and Next, disabled at the ends', async () => {
    const user = userEvent.setup()
    renderApp('/')
    await loaded()
    expect(screen.getByRole('button', { name: 'Previous' })).toBeDisabled()
    await user.click(screen.getByRole('button', { name: 'Next' }))
    expect(await screen.findByText('Page 2 of 3')).toBeInTheDocument()
    await loaded()
    expect(within(bodyRows()[0]).getAllByRole('cell')[1]).toHaveTextContent('health_beauty #0014')
    await user.click(screen.getByRole('button', { name: 'Next' }))
    expect(await screen.findByText('Page 3 of 3')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Next' })).toBeDisabled()
    await user.click(screen.getByRole('button', { name: 'Previous' }))
    expect(await screen.findByText('Page 2 of 3')).toBeInTheDocument()
  })

  it('reads the Category and page from the URL', async () => {
    renderApp('/?category=Uncategorized&page=1')
    await loaded()
    expect(productCalls()).toEqual(['/api/products?category=Uncategorized&page=1&page_size=20'])
    expect(bodyRows()).toHaveLength(3)
    expect(screen.getByRole('combobox', { name: 'Category' })).toHaveTextContent('Uncategorized')
    expect(screen.getByText('Page 1 of 1')).toBeInTheDocument()
  })

  it.each(['abc', '-2', '0', '1.5', '', '2147483648', '9999999999'])('treats page=%s as page 1', async (page) => {
    renderApp(`/?page=${page}`)
    await loaded()
    expect(productCalls()).toEqual(['/api/products?page=1&page_size=20'])
  })

  it('filters by the picked Category from page 1', async () => {
    const user = userEvent.setup()
    renderApp('/?page=2')
    await loaded()
    await user.click(screen.getByRole('combobox', { name: 'Category' }))
    const options = screen.getAllByRole('option').map((o) => o.textContent)
    expect(options).toEqual(['All Categories', 'health_beauty', 'pc_gamer', 'Uncategorized'])
    await user.click(screen.getByRole('option', { name: 'Uncategorized' }))
    await waitFor(() => expect(productCalls().at(-1)).toBe('/api/products?category=Uncategorized&page=1&page_size=20'))
    await loaded()
    expect(bodyRows()).toHaveLength(3)

    await user.click(screen.getByRole('combobox', { name: 'Category' }))
    await user.click(screen.getByRole('option', { name: 'All Categories' }))
    await waitFor(() => expect(productCalls().at(-1)).toBe('/api/products?page=1&page_size=20'))
  })

  it('says so when a Category has no Products', async () => {
    renderApp('/?category=pc_gamer')
    expect(await screen.findByText('No Products in this Category.')).toBeInTheDocument()
    expect(screen.getByText('Page 1 of 1')).toBeInTheDocument()
  })

  it('says so when the catalog is empty', async () => {
    gw.state.products = []
    renderApp('/')
    expect(await screen.findByText('No Products yet.')).toBeInTheDocument()
  })

  it('shows a failed request as a toast and stays usable', async () => {
    gw.state.fail = ['/api/products']
    renderApp('/')
    expect(await screen.findByText('Request failed: UNAVAILABLE — Service Unavailable')).toBeInTheDocument()
    await loaded()
    expect(screen.queryByText('No Products yet.')).not.toBeInTheDocument()
    gw.state.fail = []
    const user = userEvent.setup()
    await user.click(screen.getByRole('combobox', { name: 'Category' }))
    await user.click(await screen.findByRole('option', { name: 'Uncategorized' }))
    await waitFor(() => expect(bodyRows()).toHaveLength(3))
  })

  it('still lists Products when the Categories fail to load', async () => {
    gw.state.fail = ['/api/categories']
    renderApp('/')
    expect(await screen.findByText('Request failed: UNAVAILABLE — Service Unavailable')).toBeInTheDocument()
    await loaded()
    expect(bodyRows()).toHaveLength(20)
  })

  it('says when the page in the URL is past the last one', async () => {
    const user = userEvent.setup()
    renderApp('/?page=9')
    expect(await screen.findByText('This page is past the last one.')).toBeInTheDocument()
    expect(screen.getByText('Page 9 of 3')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Previous' }))
    expect(await screen.findByText('Page 3 of 3')).toBeInTheDocument()
  })

  it('disables Previous and Next until the page has loaded', () => {
    renderApp('/?page=9')
    expect(screen.getByRole('button', { name: 'Previous' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Next' })).toBeDisabled()
  })

  it('drops the page count of the previous query', async () => {
    const user = userEvent.setup()
    renderApp('/')
    expect(await screen.findByText('Page 1 of 3')).toBeInTheDocument()
    gw.state.fail = ['/api/products']
    await user.click(screen.getByRole('combobox', { name: 'Category' }))
    await user.click(screen.getByRole('option', { name: 'Uncategorized' }))
    expect(screen.queryByText('Page 1 of 3')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Next' })).toBeDisabled()
    expect(await screen.findByText('Page 1')).toBeInTheDocument()
    expect((await screen.findAllByText('Request failed: UNAVAILABLE — Service Unavailable')).length).toBeGreaterThan(0)
    expect(screen.getByRole('button', { name: 'Next' })).toBeDisabled()
  })

  it('shows an unknown Category in the URL as the gateway error', async () => {
    renderApp('/?category=nope')
    expect(await screen.findByText('Request failed: INVALID_ARGUMENT — invalid: unknown category "nope"')).toBeInTheDocument()
    // The filter still shows what the URL asked for.
    expect(screen.getByRole('combobox', { name: 'Category' })).toHaveTextContent('nope')
  })
})
