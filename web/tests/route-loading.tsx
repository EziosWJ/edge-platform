import { createRoot } from "react-dom/client";
import { createMemoryRouter, Link, Outlet, RouterProvider } from "react-router-dom";
import { RouteLoadingIndicator } from "@/components/layout/route-loading-indicator";
import { useRouteNavigationLoading } from "@/components/layout/use-route-navigation-loading";
import "@/styles/globals.css";

const wait = (milliseconds: number) => new Promise((resolve) => window.setTimeout(resolve, milliseconds));

export function Layout() {
  const { navigationIntent, handleNavigationClickCapture } = useRouteNavigationLoading();

  return (
    <div onClickCapture={handleNavigationClickCapture}>
      <Link to="/reports">打开报表</Link>
      <Link to="/instant">打开即时页面</Link>
      <RouteLoadingIndicator visible={navigationIntent} />
      <main><Outlet /></main>
    </div>
  );
}

const router = createMemoryRouter([
  {
    path: "/",
    element: <Layout />,
    children: [
      { index: true, element: <p>首页</p> },
      { path: "instant", element: <p>即时页</p> },
      {
        path: "reports",
        loader: async () => {
          await wait(300);
          return null;
        },
        element: <p>报表页</p>,
      },
    ],
  },
], { initialEntries: ["/"] });

createRoot(document.getElementById("root")!).render(<RouterProvider router={router} />);
