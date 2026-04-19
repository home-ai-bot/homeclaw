import { IconVideo, IconClock } from "@tabler/icons-react"
import { useTranslation } from "react-i18next"

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"

export function VideoSettingsSection() {
  const { t } = useTranslation("homeclaw")

  return (
    <Card className="mt-4">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <IconVideo className="size-5" />
          {t("video_section.title")}
        </CardTitle>
        <CardDescription>
          {t("video_section.comingSoon")}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
          <IconClock className="mb-2 size-12" />
          <p className="text-sm">{t("video_section.comingSoon")}</p>
        </div>
      </CardContent>
    </Card>
  )
}
