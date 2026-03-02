using Microsoft.UI;
using Microsoft.UI.Xaml.Data;
using Microsoft.UI.Xaml.Media;

namespace AgentControlWin.Converters;

public class StatusToColorConverter : IValueConverter
{
    private static readonly SolidColorBrush GreenBrush = new(Colors.LimeGreen);
    private static readonly SolidColorBrush GrayBrush = new(Windows.UI.Color.FromArgb(0xFF, 0x8a, 0x90, 0x99));

    public object Convert(object value, Type targetType, object parameter, string language)
    {
        var status = value as string ?? "";
        var isOnline = status == "online" || status == "running";
        return isOnline ? GreenBrush : GrayBrush;
    }

    public object ConvertBack(object value, Type targetType, object parameter, string language)
        => throw new NotImplementedException();
}

public class OnlineToBrushConverter : IValueConverter
{
    private static readonly SolidColorBrush SuccessBrush = new(Windows.UI.Color.FromArgb(0xFF, 0x3a, 0x9e, 0x74));
    private static readonly SolidColorBrush GrayBrush = new(Windows.UI.Color.FromArgb(0x99, 0x8a, 0x90, 0x99));

    public object Convert(object value, Type targetType, object parameter, string language)
        => value is bool b && b ? SuccessBrush : GrayBrush;

    public object ConvertBack(object value, Type targetType, object parameter, string language)
        => throw new NotImplementedException();
}

public class ChatRunStateToColorConverter : IValueConverter
{
    private static readonly SolidColorBrush AccentBrush = new(Windows.UI.Color.FromArgb(0xFF, 0x31, 0x5f, 0x72));
    private static readonly SolidColorBrush WarningBrush = new(Windows.UI.Color.FromArgb(0xFF, 0xc8, 0x75, 0x33));
    private static readonly SolidColorBrush DangerBrush = new(Windows.UI.Color.FromArgb(0xFF, 0xd9, 0x4f, 0x4f));
    private static readonly SolidColorBrush TransparentBrush = new(Colors.Transparent);

    public object Convert(object value, Type targetType, object parameter, string language)
    {
        if (value is AgentControlWin.Core.ChatRunState state)
            return state switch
            {
                AgentControlWin.Core.ChatRunState.Running => AccentBrush,
                AgentControlWin.Core.ChatRunState.Slow => WarningBrush,
                AgentControlWin.Core.ChatRunState.Error => DangerBrush,
                _ => TransparentBrush,
            };
        return TransparentBrush;
    }

    public object ConvertBack(object value, Type targetType, object parameter, string language)
        => throw new NotImplementedException();
}
